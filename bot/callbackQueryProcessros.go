package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"telebot/aiApi"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func processCallbackQuery(ctx context.Context, b *bot.Bot, update *models.Update) {

	msgId := update.CallbackQuery.InlineMessageID
	processImgGenerationError := func() error {
		_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			InlineMessageID: msgId,
			Text:            "Нет, сервер подох!",
		})
		return err
	}

	queryData, ok := queries.getValue(update.CallbackQuery.Data)
	if !ok {
		log.Printf("[error] error getting inline query by key %s\n", update.CallbackQuery.Data)
		processImgGenerationError()
		return
	}

	url, err := aiApi.GetImage(queryData.query)
	if err != nil {
		log.Println("[error] error generating image")
		processImgGenerationError()
		return
	}

	response, e := http.Get(url)
	if e != nil {
		defer response.Body.Close()
		log.Println("[error] error getting generated image")
		processImgGenerationError()
		return
	}

	defer response.Body.Close()

	imageBytes, e := io.ReadAll(response.Body)
	if e != nil {
		log.Println("[error] error reading generated image")
		processImgGenerationError()
		return
	}

	b.SendPhoto(ctx, &bot.SendPhotoParams{})

	res, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID: os.Getenv("TMP_CHAT_ID"),
		Photo:  &models.InputFileUpload{Filename: "photo", Data: bytes.NewReader(imageBytes)},
	})
	if err != nil {
		log.Println(err)
		processImgGenerationError()
		return
	}

	if len(res.Photo) == 0 {
		log.Println("Can't get uploaded image")
		processImgGenerationError()
		return
	}

	photo := &models.InputMediaPhoto{
		Media:      res.Photo[0].FileID,
		HasSpoiler: true,
		Caption:    fmt.Sprintf("Нарисовать %s", queryData.query),
	}

	_, err = b.EditMessageMedia(ctx, &bot.EditMessageMediaParams{
		InlineMessageID: update.CallbackQuery.InlineMessageID,
		Media:           photo,
	})

	if err != nil {
		var typeErr *json.UnmarshalTypeError
		if !errors.As(err, &typeErr) {
			log.Println("[error] error editing message")
			log.Println(err)
			processImgGenerationError()
		}
	}

	_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    res.Chat.ID,
		MessageID: res.ID,
	})
	if err != nil {
		log.Println("[error] error deleting message")
		log.Println(err)
		processImgGenerationError()
		return
	}

	queries.deleteValue(update.CallbackQuery.Data)
	queries.deleteOldValues()
}
