package bot

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"telebot/aiApi"
	"telebot/model"
	"telebot/utils"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// stickerTitleFallback names a pack whose owner gave us nothing to name it by.
const stickerTitleFallback = "Emotions"

// stickerTitleLimit is Telegram's ceiling for a set title.
const stickerTitleLimit = 64

// stickerSetTitle is what the person sees above the pack in the sticker panel,
// and what everyone else sees when they tap a sticker from it.
//
// The name of the owner belongs here: a person who adds packs of three friends
// would otherwise get three sets called the same thing, told apart only by the
// pictures.
//
// The word itself is English so that the title reads the same for everyone,
// whatever language they speak — a sticker travels far beyond the chat it was
// made in. The name comes after a colon and is never inflected: "Emotions:
// Вася" works for a name in any script, while a possessive would have to guess
// grammar from a Telegram profile, and profiles hold anything from "Вася" to
// an emoji.
func stickerSetTitle(from *models.User) string {
	if from == nil {
		return stickerTitleFallback
	}

	name := strings.TrimSpace(from.FirstName)
	if name == "" {
		name = strings.TrimSpace(from.Username)
	}
	if name == "" {
		return stickerTitleFallback
	}

	title := "Emotions: " + name
	if len([]rune(title)) > stickerTitleLimit {
		title = string([]rune(title)[:stickerTitleLimit])
	}

	return title
}

// stickerNoPhotoText is the answer to a bare command. The pack is built from a
// face, and there is nothing to guess from.
const stickerNoPhotoText = "Пришли фотографию с лицом — или ответь этой командой " +
	"на уже присланную, и я сделаю из неё набор стикеров."

// stickerBusyText warns about the wait. Ten pictures at around half a minute
// each is minutes, and a person who is not told that decides the bot is dead.
const stickerBusyText = "Делаю набор стикеров. Это займёт несколько минут — " +
	"я буду показывать, сколько готово."

// stickerPackPrefix starts the set's short name. Telegram requires the name to
// end with _by_<bot>, to be unique across the whole platform and to consist of
// latin letters, digits and underscores.
const stickerPackPrefix = "emo"

// stickerPackName builds the set's short name for a person. The user id makes
// it unique without a counter, and the same person always gets the same name —
// that is what lets a regeneration refill the existing set instead of leaving
// a trail of abandoned ones.
func stickerPackName(userID int64) string {
	return fmt.Sprintf("%s%d_by_%s", stickerPackPrefix, userID, botName)
}

// stickerPackLink is the address a person opens to add their pack.
func stickerPackLink(name string) string {
	return "https://t.me/addstickers/" + name
}

// processStickerPack builds the standard pack out of one photo.
//
// The picture is taken from the message itself or from the one it replies to,
// the same way the other picture commands do it: people attach the command to
// a photo about as often as they send both at once.
func processStickerPack(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	chatID := message.Chat.ID

	reply := func(text string) {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatID,
			Text:            text,
			ReplyParameters: &models.ReplyParameters{MessageID: message.ID},
		})
		utils.ProcessSendMessageError(err, chatID)
	}

	photos := message.Photo
	if len(photos) == 0 && message.ReplyToMessage != nil {
		photos = message.ReplyToMessage.Photo
	}
	if len(photos) == 0 {
		reply(stickerNoPhotoText)
		return
	}

	biggest := getBiggestPhoto(photos)
	if biggest == nil {
		reply(stickerNoPhotoText)
		return
	}

	imageBytes, name, err := downloadTelegramFile(ctx, b, biggest.FileID)
	if err != nil {
		log.Println("[error] sticker pack: cannot download the photo", err)
		reply(serverDeadText)
		return
	}

	wait := newChatWaitMessage(ctx, b, chatID, message.ID)
	wait.setText(stickerBusyText)

	total := aiApi.StickerPackSize()
	caller := messageCaller(message, wait)
	// The pack is long, and the queue position of each separate picture means
	// nothing to the person waiting. What matters is how many are ready, so the
	// wait message shows that instead.
	caller.Progress = nil

	stickers, err := aiApi.GenerateStickers(
		aiApi.EditImage{Bytes: imageBytes, Name: name}, caller,
		func(done, all int) {
			wait.setText(fmt.Sprintf("%s\n\nГотово %d из %d.", stickerBusyText, done, all))
		})
	wait.done()

	if err != nil {
		log.Println("[error] sticker pack failed:", err)
		b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: wait.id()})
		reply(generationErrorText(err))
		return
	}

	link, err := publishStickerPack(ctx, b, message.From, stickers)
	b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: wait.id()})
	if err != nil {
		log.Println("[error] sticker pack not published:", err)
		reply(serverDeadText)
		return
	}

	short := ""
	if len(stickers) < total {
		short = fmt.Sprintf("\n\nПолучилось %d из %d — остальные не вышли.",
			len(stickers), total)
	}
	reply(fmt.Sprintf("Готово! Набор здесь: %s%s", link, short))
}

// uploadSticker hands one picture to Telegram and returns the file id to
// refer to it by.
//
// The upload is a separate call on purpose, and it is the only way that works.
// The library builds multipart out of the TOP-LEVEL fields of a params struct:
// an *InputFileUpload lying directly in a field becomes an attached file,
// while the same thing inside Stickers []InputSticker goes through plain JSON
// and marshals to the string "@name.png". Telegram then reads that as a URL
// and answers "failed to get HTTP URL content" — which is exactly what the
// first live run got. In UploadStickerFileParams the picture IS a top-level
// field, so it uploads properly, and the file id it returns can be put into
// the set.
func uploadSticker(ctx context.Context, b *bot.Bot, userID int64, index int,
	png []byte) (string, error) {
	file, err := b.UploadStickerFile(ctx, &bot.UploadStickerFileParams{
		UserID: userID,
		Sticker: &models.InputFileUpload{
			Filename: fmt.Sprintf("sticker%d.png", index),
			Data:     bytes.NewReader(png),
		},
		StickerFormat: "static",
	})
	if err != nil {
		return "", err
	}

	return file.FileID, nil
}

// publishStickerPack creates the person's set or refills the one they already
// have, and returns the link to it.
//
// Refilling means deleting the old stickers and adding the new ones. It is
// done in that order on purpose: Telegram refuses to delete the last sticker
// of a set, so the new ones go in first and the old ones leave afterwards,
// which also means the set is never empty in between.
func publishStickerPack(ctx context.Context, b *bot.Bot, from *models.User,
	stickers []aiApi.StickerResult) (string, error) {
	userID := from.ID
	name := stickerPackName(userID)

	existing, err := model.GetStickerPack(userID)
	if err != nil {
		// Not knowing what the person had is not a reason to refuse: try to
		// create, and if the set is already there, fall through to refilling.
		log.Println("[warn] sticker pack lookup failed:", err)
	}

	if existing == nil {
		inputs := make([]models.InputSticker, 0, len(stickers))
		for i, s := range stickers {
			fileID, err := uploadSticker(ctx, b, userID, i, s.PNG)
			if err != nil {
				log.Printf("[warn] sticker %s not uploaded: %v\n", s.Emotion.Key, err)
				continue
			}

			inputs = append(inputs, models.InputSticker{
				Sticker:   &models.InputFileString{Data: fileID},
				Format:    "static",
				EmojiList: s.Emotion.Emoji,
			})
		}
		if len(inputs) == 0 {
			return "", fmt.Errorf("no stickers uploaded for set %s", name)
		}

		ok, err := b.CreateNewStickerSet(ctx, &bot.CreateNewStickerSetParams{
			UserID:   userID,
			Name:     name,
			Title:    stickerSetTitle(from),
			Stickers: inputs,
		})
		if err != nil || !ok {
			// A set with this name may survive a database we have lost. In that
			// case creating fails and refilling is the right move.
			if !isStickerSetExists(err) {
				return "", fmt.Errorf("creating sticker set %s: %w", name, err)
			}
		} else {
			if err := model.SaveStickerPack(userID, name, len(stickers)); err != nil {
				log.Println("[warn] sticker pack not saved:", err)
			}

			return stickerPackLink(name), nil
		}
	}

	set, err := b.GetStickerSet(ctx, &bot.GetStickerSetParams{Name: name})
	if err != nil {
		return "", fmt.Errorf("reading sticker set %s: %w", name, err)
	}

	added := 0
	for i, s := range stickers {
		fileID, err := uploadSticker(ctx, b, userID, i, s.PNG)
		if err != nil {
			log.Printf("[warn] sticker %s not uploaded: %v\n", s.Emotion.Key, err)
			continue
		}

		if _, err := b.AddStickerToSet(ctx, &bot.AddStickerToSetParams{
			UserID: userID,
			Name:   name,
			Sticker: models.InputSticker{
				Sticker:   &models.InputFileString{Data: fileID},
				Format:    "static",
				EmojiList: s.Emotion.Emoji,
			},
		}); err != nil {
			log.Printf("[warn] sticker %s not added: %v\n", s.Emotion.Key, err)
			continue
		}
		added++
	}

	// Старые сносим, только если новые вправду легли: иначе от набора остался
	// бы один стикер, который Telegram и удалить не даст.
	if added == 0 {
		return "", fmt.Errorf("nothing added to set %s", name)
	}

	for _, old := range set.Stickers {
		if _, err := b.DeleteStickerFromSet(ctx, &bot.DeleteStickerFromSetParams{
			Sticker: old.FileID,
		}); err != nil {
			log.Println("[warn] old sticker not deleted:", err)
		}
	}

	if err := model.SaveStickerPack(userID, name, len(stickers)); err != nil {
		log.Println("[warn] sticker pack not saved:", err)
	}

	return stickerPackLink(name), nil
}

// isStickerSetExists tells the "this name is taken" refusal from the rest.
// Telegram has no code for it, only the description, so the text is what we
// look at.
func isStickerSetExists(err error) bool {
	if err == nil {
		return false
	}

	text := strings.ToLower(err.Error())

	return strings.Contains(text, "sticker set name is already occupied") ||
		strings.Contains(text, "sticker set already exists")
}

// stickerNoPackText answers someone who has nothing to delete.
const stickerNoPackText = "У тебя ещё нет набора. Пришли фото с подписью " +
	"/stickers — сделаю."

// processStickerPackDelete removes the person's set from Telegram entirely.
//
// Deleting is the bot's job, not the person's: a set made by a bot cannot be
// edited from the Telegram interface, only hidden from one's own list, and
// hiding leaves it alive and re-addable by the old link.
//
// The record goes away even when Telegram says there is no such set. That
// happens when the set was already deleted elsewhere, and keeping a row that
// points at nothing would only make the next /stickers try to refill a ghost.
func processStickerPackDelete(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	chatID := message.Chat.ID

	reply := func(text string) {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatID,
			Text:            text,
			ReplyParameters: &models.ReplyParameters{MessageID: message.ID},
		})
		utils.ProcessSendMessageError(err, chatID)
	}

	userID := message.From.ID
	pack, err := model.GetStickerPack(userID)
	if err != nil {
		log.Println("[error] sticker pack lookup failed:", err)
		reply(serverDeadText)
		return
	}
	if pack == nil {
		reply(stickerNoPackText)
		return
	}

	_, err = b.DeleteStickerSet(ctx, &bot.DeleteStickerSetParams{Name: pack.Name})
	if err != nil && !isStickerSetMissing(err) {
		log.Println("[error] sticker set not deleted:", err)
		reply(serverDeadText)
		return
	}

	if err := model.DeleteStickerPack(userID); err != nil {
		log.Println("[error] sticker pack record not deleted:", err)
		reply(serverDeadText)
		return
	}

	reply("Набор удалён. Захочешь новый — пришли фото с подписью /stickers.")
}

// isStickerSetMissing recognizes "there is no such set" among other refusals.
// Telegram gives no code for it, only the description, so the text is what we
// look at.
func isStickerSetMissing(err error) bool {
	if err == nil {
		return false
	}

	text := strings.ToLower(err.Error())

	return strings.Contains(text, "stickerset_invalid") ||
		strings.Contains(text, "sticker set not found") ||
		strings.Contains(text, "sticker set is invalid")
}
