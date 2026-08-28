package bot

import (
	"context"
	"log"
	"os"
	"strings"
	"telebot/raffleLogic"
	"telebot/utils"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var botName string = ""

var queries = safeQueryMap{
	value: make(map[string]callbackQueryData),
}

// getHandler returns the default update handler. Generation runs inline:
// the library already gives every update its own goroutine, and queueing is
// the generation service's job, so there is nothing to hand off to here.
func getHandler() bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {

		sendWaitInlineQueryMessage := func(msgId string) {
			if err := editInlineStatus(ctx, b, msgId, "ОЖИДАЕМ!!!"); err != nil {
				log.Println("[error] error setting the inline wait status")
				log.Println(err)
			}
		}

		sendWaitMessage := func(chatId int64, replyToMessageId int) int {
			msg, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          chatId,
				Text:            "Ладно",
				ReplyParameters: &models.ReplyParameters{MessageID: replyToMessageId, ChatID: chatId},
			})
			utils.ProcessSendMessageError(err, chatId)

			time.Sleep(2 * time.Second)

			_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
				MessageID: msg.ID,
				ChatID:    chatId,
				Text:      "Жди теперь",
			})
			utils.ProcessSendMessageError(err, chatId)

			return msg.ID
		}

		if update.InlineQuery != nil {
			saveUser(update.InlineQuery.From)
			processInlineQuery(ctx, b, update)
		} else if update.CallbackQuery != nil {
			saveUser(&update.CallbackQuery.From)
			userName := utils.GetAnyName(&update.CallbackQuery.From)

			// Telegram spins a clock on the button until the press is
			// acknowledged, and generation takes far longer than it waits.
			if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: update.CallbackQuery.ID,
			}); err != nil {
				log.Println("[error] error answering callback query")
				log.Println(err)
			}

			queryData, ok := queries.getValue(update.CallbackQuery.Data)
			if !ok {
				// There is nothing left to generate; processCallbackQuery says
				// so on the message itself.
				processCallbackQuery(ctx, b, update)
				return
			}

			sendWaitInlineQueryMessage(update.CallbackQuery.InlineMessageID)

			if queryData.kind.isVideo() {
				log.Println("Inline video generation requested by", userName)
			} else {
				log.Println("Image generation requested by", userName)
			}
			processCallbackQuery(ctx, b, update)
		} else if update.Message != nil {
			chatId := update.Message.Chat.ID
			saveUser(update.Message.From)
			userName := utils.GetAnyName(update.Message.From)
			log.Println("Received message from", userName)

			msgTextLower := strings.ToLower(getMessageText(update.Message))

			imgPrompt := ""
			if isCommand(msgTextLower, "нарисуй", "draw") {
				imgPrompt = buildAiPrompt(update.Message, "нарисуй", "draw")
			}

			if imgPrompt != "" {
				log.Println("Image generation requested by", userName)
				mainMessageId := sendWaitMessage(chatId, update.Message.ID)
				processImageGeneration(ctx, b, update, mainMessageId, imgPrompt)
			} else if isCommand(msgTextLower, "анимируй", "animate") {
				log.Println("Video generation requested by", userName)
				mainMessageId := sendWaitMessage(chatId, update.Message.ID)
				processVideoGeneration(ctx, b, update, mainMessageId, buildAiPrompt(update.Message, "анимируй", "animate"))
			} else if isCommand(msgTextLower, "расшифруй", "transcribe") {
				log.Println("Transcription requested by", userName)
				mainMessageId := sendWaitMessage(chatId, update.Message.ID)
				processTranscription(ctx, b, update, mainMessageId)
			} else if strings.HasPrefix(msgTextLower, "/ai_help") || strings.HasPrefix(msgTextLower, "/ai_help@"+botName) {
				log.Println("AI help requested by", userName)
				processAIHelp(ctx, b, update)
			} else if strings.HasPrefix(msgTextLower, "/help") || strings.HasPrefix(msgTextLower, "/help@"+botName) {
				log.Println("Help requested by", userName)
				processHelp(ctx, b, update)
			} else if strings.HasPrefix(msgTextLower, "/forget") || strings.HasPrefix(msgTextLower, "/forget@"+botName) {
				log.Println("Forgetting inline images requested by", userName)
				processForgetInlineImages(ctx, b, update)
			} else if update.Message.Chat.Type == "private" && strings.HasPrefix(msgTextLower, "/start "+inlineImageUploadStartParam) {
				log.Println("Inline image upload started by", userName)
				processInlineImageUploadStart(ctx, b, update)
			} else if update.Message.Chat.Type == "private" && len(update.Message.Photo) > 0 {
				log.Println("Inline image sent by", userName)
				processInlineImageUpload(ctx, b, update)
			}

			msgType := update.Message.Chat.Type
			if msgType != "group" && msgType != "supergroup" {
				return
			}

			chat, err := saveChat(update)
			if err != nil {
				return
			}
			syncSuperAdmins(ctx, b, update)

			if !raffleLogic.IsNoReturnPoint() {
				log.Println("Running processParticipation")
				processParticipation(update)
			}

			if msgTextLower == "/stats" || strings.HasPrefix(msgTextLower, "/stats@"+botName) {
				log.Println("Stats requested by", userName)
				processStats(ctx, b, update, false)
			} else if msgTextLower == "/stats_full" || strings.HasPrefix(msgTextLower, "/stats_full@"+botName) {
				log.Println("Full stats requested by", userName)
				processStats(ctx, b, update, true)
			} else if strings.HasPrefix(msgTextLower, "сегодня ") || strings.HasPrefix(msgTextLower, "завтра ") {
				log.Println("Prize requested by", userName)
				processPrize(ctx, b, update, chat)
			} else if msgTextLower == "/prize" || strings.HasPrefix(msgTextLower, "/prize@"+botName) {
				log.Println("Prize info requested by", userName)
				processPrizeInfo(ctx, b, chat)
			} else if strings.HasPrefix(msgTextLower, "/set_admin") || strings.HasPrefix(msgTextLower, "/set_admin@"+botName) {
				log.Println("Setting admin requested by", userName)
				processSetAdmin(ctx, b, update)
			} else if strings.HasPrefix(msgTextLower, "/unset_admin") || strings.HasPrefix(msgTextLower, "/unset_admin@"+botName) {
				log.Println("Unsetting admin requested by", userName)
				processUnsetAdmin(ctx, b, update)
			} else if strings.HasPrefix(msgTextLower, "/admins") || strings.HasPrefix(msgTextLower, "/admins@"+botName) {
				log.Println("Admins requested by", userName)
				processAdmins(ctx, b, update)
			}
		}
	}
}

func New(ctx context.Context) *bot.Bot {
	opts := []bot.Option{
		bot.WithDefaultHandler(getHandler()),
		bot.WithAllowedUpdates([]string{"callback_query", "message", "inline_query"}),
	}

	b, err := bot.New(os.Getenv("TELEGRAM_APITOKEN"), opts...)
	if err != nil {
		panic(err)
	}

	self, err := b.GetMe(ctx)
	if err != nil {
		panic(err)
	}
	botName = self.Username

	return b
}

func Start(ctx context.Context, b *bot.Bot) {
	b.Start(ctx)
}
