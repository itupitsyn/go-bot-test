package bot

import (
	"context"
	"fmt"
	"log"
	"sync"
	"telebot/model"
	"telebot/utils"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// inlineImageUploadStartParam is the payload of the deep link behind the button
// shown above inline results, so that "/start upload" in the private chat can be
// told apart from a plain start.
const inlineImageUploadStartParam = "upload"

// inlineImagesCleanup throttles the lazy cleanup: pictures are dropped off user
// actions rather than by a timer, and there is no point in running the delete
// more than once an hour no matter how many pictures somebody uploads.
var inlineImagesCleanup = struct {
	mutex sync.Mutex
	last  time.Time
}{}

// cleanupInlineImages drops pictures nobody has touched for model.InlineImageTTL.
// Rotation already caps the buffer of an active user, so this only collects
// after the ones who tried the bot once and never came back.
func cleanupInlineImages() {
	inlineImagesCleanup.mutex.Lock()
	if time.Since(inlineImagesCleanup.last) < time.Hour {
		inlineImagesCleanup.mutex.Unlock()
		return
	}
	inlineImagesCleanup.last = time.Now()
	inlineImagesCleanup.mutex.Unlock()

	deleted, err := model.DeleteOldInlineImages()
	if err != nil {
		log.Println("[error] error deleting old inline images", err)
		return
	}
	if deleted > 0 {
		log.Printf("Deleted %d inline images untouched for %s\n", deleted, model.InlineImageTTL)
	}
}

// inlineImageBufferHint explains the rotation rules, so that a picture quietly
// disappearing from the inline list later does not look like a bug.
func inlineImageBufferHint() string {
	return fmt.Sprintf("Держу последние %d картинок — новая вытесняет самую старую. "+
		"Нетронутые пропадают через %d дней. /forget — забыть всё сразу.",
		model.InlineImageLimit, int(model.InlineImageTTL.Hours()/24))
}

// processInlineImageUpload remembers a picture sent to the bot in private, so
// that it can be animated from inline mode later. Only the file id is stored.
func processInlineImageUpload(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := update.Message.Chat.ID
	photo := getBiggestPhoto(update.Message.Photo)
	if photo == nil {
		return
	}

	// The biggest size goes to the animator, the smallest one is what the
	// preview endpoint serves to inline results.
	thumb := getSmallestPhoto(update.Message.Photo)

	if err := model.SaveInlineImage(update.Message.From.ID, photo.FileID, thumb.FileID, photo.FileUniqueID); err != nil {
		log.Println("[error] error saving inline image", err)
		_, botErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatId,
			Text:            "Не смог запомнить картинку, попробуй ещё разок",
			ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
		})
		utils.ProcessSendMessageError(botErr, chatId)
		return
	}

	go cleanupInlineImages()

	msgText := fmt.Sprintf("Запомнил! Набери в любом чате @%s и выбери «Анимировать мою картинку».\n\n%s",
		botName, inlineImageBufferHint())
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatId,
		Text:            msgText,
		ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
	})
	utils.ProcessSendMessageError(err, chatId)
}

// processInlineImageUploadStart greets the user who got here through the button
// above inline results.
func processInlineImageUploadStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := update.Message.Chat.ID
	msgText := fmt.Sprintf("Пришли мне сюда картинку — и когда наберёшь @%s в любом чате, в списке появится "+
		"«Анимировать мою картинку». Оживлю её прямо там, даже если меня в том чате нет.\n\n%s",
		botName, inlineImageBufferHint())

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatId,
		Text:   msgText,
	})
	utils.ProcessSendMessageError(err, chatId)
}

// processForgetInlineImages empties the user's buffer on demand.
func processForgetInlineImages(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := update.Message.Chat.ID

	deleted, err := model.DeleteInlineImagesByUser(update.Message.From.ID)
	if err != nil {
		log.Println("[error] error deleting inline images", err)
		_, botErr := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatId,
			Text:            "Не вышло, сервер подох",
			ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
		})
		utils.ProcessSendMessageError(botErr, chatId)
		return
	}

	msgText := "Нечего забывать, картинок и не было"
	if deleted > 0 {
		msgText = fmt.Sprintf("Забыл %d шт. Как будто и не было", deleted)
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatId,
		Text:            msgText,
		ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
	})
	utils.ProcessSendMessageError(err, chatId)
}
