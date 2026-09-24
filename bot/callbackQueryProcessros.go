package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"telebot/aiApi"
	"telebot/model"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// inlineUploadAttempts is how many times the generated media is pushed to the
// service chat before giving up.
const inlineUploadAttempts = 3

// ignoreInlineEditResponse drops the error every successful edit of an inline
// message produces. Telegram answers such an edit with "true" instead of the
// edited message, and the library fails to unmarshal that into models.Message —
// the edit itself went through.
func ignoreInlineEditResponse(err error) error {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return nil
	}

	return err
}

// editInlineStatus reports progress or failure on the inline message the pressed
// button belongs to. Every inline result we answer with is an article, so the
// message carries text rather than a caption. The edit passes no reply markup,
// which drops the button along the way — handy, since a second press would kick
// off a second generation.
func editInlineStatus(ctx context.Context, b *bot.Bot, inlineMessageId string, text string) error {
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		InlineMessageID: inlineMessageId,
		Text:            text,
	})
	return ignoreInlineEditResponse(err)
}

// uploadToServiceChat parks generated media in the service chat to get a file id
// out of it: editing an inline message cannot upload anything, it only accepts a
// file id Telegram already knows or a URL.
func uploadToServiceChat(send func() (*models.Message, error)) (*models.Message, error) {
	var err error
	for i := 0; i < inlineUploadAttempts; i++ {
		var res *models.Message
		res, err = send()
		if err == nil {
			return res, nil
		}
		log.Println(err)
	}

	return nil, err
}

// replaceInlineMedia swaps the inline message for the generated media and then
// removes the copy in the service chat it was uploaded through.
func replaceInlineMedia(ctx context.Context, b *bot.Bot, inlineMessageId string, media models.InputMedia, uploaded *models.Message) error {
	_, err := b.EditMessageMedia(ctx, &bot.EditMessageMediaParams{
		InlineMessageID: inlineMessageId,
		Media:           media,
	})
	if err := ignoreInlineEditResponse(err); err != nil {
		return fmt.Errorf("editing inline message: %w", err)
	}

	_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    uploaded.Chat.ID,
		MessageID: uploaded.ID,
	})
	if err != nil {
		// The result is already in place, so a leftover in the service chat is
		// not worth showing the user an error over.
		log.Println("[error] error deleting message from the service chat")
		log.Println(err)
	}

	return nil
}

// generateInlineImage draws the picture asked for by an inline query and puts it
// into the message the button was pressed in.
func generateInlineImage(ctx context.Context, b *bot.Bot, inlineMessageId string, queryData callbackQueryData, userID int64) error {
	wait := newInlineWaitMessage(ctx, b, inlineMessageId)
	imageBytes, err := aiApi.GetImage(queryData.query, inlineCaller(userID, wait))
	wait.done()
	if err != nil {
		return fmt.Errorf("generating image: %w", err)
	}

	uploaded, err := uploadToServiceChat(func() (*models.Message, error) {
		return b.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID: os.Getenv("TMP_CHAT_ID"),
			Photo:  &models.InputFileUpload{Filename: "photo", Data: bytes.NewReader(imageBytes)},
		})
	})
	if err != nil {
		return fmt.Errorf("uploading generated image: %w", err)
	}

	if len(uploaded.Photo) == 0 {
		return errors.New("can't get uploaded image")
	}

	media := &models.InputMediaPhoto{
		Media:      uploaded.Photo[0].FileID,
		HasSpoiler: true,
		Caption:    queryData.query,
	}

	return replaceInlineMedia(ctx, b, inlineMessageId, media, uploaded)
}

// uploadedVideoMedia describes the parked video the way Telegram handed it back:
// a soundless mp4 comes back as an animation now and then, and the media type
// has to match the file id or the edit is rejected.
func uploadedVideoMedia(uploaded *models.Message, caption string) models.InputMedia {
	if uploaded.Video != nil {
		return &models.InputMediaVideo{Media: uploaded.Video.FileID, HasSpoiler: true, Caption: caption}
	}
	if uploaded.Animation != nil {
		return &models.InputMediaAnimation{Media: uploaded.Animation.FileID, HasSpoiler: true, Caption: caption}
	}

	return nil
}

// putInlineVideo parks the generated video in the service chat and swaps the
// inline message for it.
func putInlineVideo(ctx context.Context, b *bot.Bot, inlineMessageId string, videoBytes []byte, caption string) error {
	uploaded, err := uploadToServiceChat(func() (*models.Message, error) {
		return b.SendVideo(ctx, &bot.SendVideoParams{
			ChatID: os.Getenv("TMP_CHAT_ID"),
			Video:  &models.InputFileUpload{Filename: "video.mp4", Data: bytes.NewReader(videoBytes)},
		})
	})
	if err != nil {
		return fmt.Errorf("uploading generated video: %w", err)
	}

	media := uploadedVideoMedia(uploaded, caption)
	if media == nil {
		return errors.New("can't get uploaded video")
	}

	return replaceInlineMedia(ctx, b, inlineMessageId, media, uploaded)
}

// generateInlineTextVideo animates the words of the inline query, with no
// picture to start from.
func generateInlineTextVideo(ctx context.Context, b *bot.Bot, inlineMessageId string, queryData callbackQueryData, userID int64) error {
	wait := newInlineWaitMessage(ctx, b, inlineMessageId)
	videoBytes, err := aiApi.GetT2V(queryData.query, inlineCaller(userID, wait))
	wait.done()
	if err != nil {
		return fmt.Errorf("generating t2v: %w", err)
	}

	return putInlineVideo(ctx, b, inlineMessageId, videoBytes, queryData.query)
}

// generateInlineVideo animates the picture behind the pressed button and puts
// the result into the message the picture was posted in.
func generateInlineVideo(ctx context.Context, b *bot.Bot, inlineMessageId string, queryData callbackQueryData, userID int64) error {
	imageBytes, imageName, err := downloadTelegramFile(ctx, b, queryData.fileID)
	if err != nil {
		// Telegram no longer serves the picture, so it should stop being
		// offered in inline results.
		if deleteErr := model.DeleteInlineImage(queryData.ownerID, queryData.fileID); deleteErr != nil {
			log.Println("[error] error deleting dead inline image", deleteErr)
		}
		return fmt.Errorf("getting image for inline i2v: %w", err)
	}

	prompt := queryData.query
	if prompt == "" {
		prompt = defaultAnimationPrompt
	}

	wait := newInlineWaitMessage(ctx, b, inlineMessageId)
	videoBytes, err := aiApi.GetI2V(prompt, imageBytes, imageName, inlineCaller(userID, wait))
	wait.done()
	if err != nil {
		return fmt.Errorf("generating i2v: %w", err)
	}

	return putInlineVideo(ctx, b, inlineMessageId, videoBytes, queryData.query)
}

func processCallbackQuery(ctx context.Context, b *bot.Bot, update *models.Update) {
	inlineMessageId := update.CallbackQuery.InlineMessageID

	queryData, ok := queries.getValue(update.CallbackQuery.Data)
	if !ok {
		log.Printf("[error] error getting inline query by key %s\n", update.CallbackQuery.Data)
		editInlineStatus(ctx, b, inlineMessageId, "Нет, сервер подох!")
		return
	}

	// The cap and the round-robin are counted against whoever pressed the
	// button: they are the one loading the GPU.
	userID := update.CallbackQuery.From.ID

	var err error
	switch queryData.kind {
	case callbackQueryImageVideo:
		err = generateInlineVideo(ctx, b, inlineMessageId, queryData, userID)
	case callbackQueryTextVideo:
		err = generateInlineTextVideo(ctx, b, inlineMessageId, queryData, userID)
	default:
		err = generateInlineImage(ctx, b, inlineMessageId, queryData, userID)
	}

	if err != nil {
		log.Println("[error] error processing inline generation")
		log.Println(err)
		text := "Нет, сервер подох!"
		if errors.Is(err, aiApi.ErrQueueFull) {
			text = queueFullText
		}
		editInlineStatus(ctx, b, inlineMessageId, text)
	}

	queries.deleteValue(update.CallbackQuery.Data)
	queries.deleteOldValues()
}
