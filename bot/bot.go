package bot

import (
	"context"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"telebot/raffleLogic"
	"telebot/utils"

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
			if err := editInlineStatus(ctx, b, msgId, inlineOkText); err != nil {
				log.Println("[error] error setting the inline wait status")
				log.Println(err)
			}
		}

		sendWaitMessage := func(chatId int64, replyToMessageId int) *waitMessage {
			return newChatWaitMessage(ctx, b, chatId, replyToMessageId)
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

			// На профилактике кнопку не трогаем в сервис, а говорим об этом
			// прямо в сообщении. Правка текста заодно снимает кнопку, так что
			// и запись под неё больше не нужна.
			if text, ok := activeMaintenanceText(); ok {
				log.Println("Inline AI generation refused: maintenance")
				if err := editInlineStatus(ctx, b, update.CallbackQuery.InlineMessageID, text); err != nil {
					log.Println("[error] error setting the inline maintenance status")
					log.Println(err)
				}
				queries.deleteValue(update.CallbackQuery.Data)
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

			isDraw := isCommand(msgTextLower, "нарисуй", "draw")
			imgPrompt := ""
			if isDraw {
				imgPrompt = buildAiPrompt(update.Message, "нарисуй", "draw")
			}

			// Та же команда, но с картинкой — это правка, а не генерация с
			// нуля: «нарисуй ей рыжие волосы» в ответ на фото. Промпт там
			// значит другое (инструкция, а не описание кадра), поэтому и ручка
			// другая.
			editImages := []*models.PhotoSize(nil)
			if isDraw {
				editImages = editPhotos(update.Message)
			}

			if isDraw && len(editImages) > 0 {
				if imgPrompt == "" {
					processEditHint(ctx, b, update)
				} else {
					log.Println("Image edit requested by", userName)
					if !replyIfMaintenance(ctx, b, update.Message) {
						wait := sendWaitMessage(chatId, update.Message.ID)
						processImageEdit(ctx, b, update, wait, imgPrompt, editImages)
					}
				}
			} else if imgPrompt != "" {
				log.Println("Image generation requested by", userName)
				if !replyIfMaintenance(ctx, b, update.Message) {
					wait := sendWaitMessage(chatId, update.Message.ID)
					processImageGeneration(ctx, b, update, wait, imgPrompt)
				}
			} else if isCommand(msgTextLower, "анимируй", "animate") {
				log.Println("Video generation requested by", userName)
				if !replyIfMaintenance(ctx, b, update.Message) {
					wait := sendWaitMessage(chatId, update.Message.ID)
					processVideoGeneration(ctx, b, update, wait, buildAiPrompt(update.Message, "анимируй", "animate"))
				}
			} else if isCommand(msgTextLower, "расшифруй", "transcribe") {
				log.Println("Transcription requested by", userName)
				if !replyIfMaintenance(ctx, b, update.Message) {
					wait := sendWaitMessage(chatId, update.Message.ID)
					processTranscription(ctx, b, update, wait)
				}
			} else if isBareCommand(msgTextLower, "что тут") || isBareCommand(msgTextLower, "сократи", "summarize", "tldr") {
				log.Println("Summary requested by", userName)
				processSummary(ctx, b, update)
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

// recoverPanics keeps one bad update from taking the whole bot down. Handlers
// get a goroutine each and a panic in a goroutine kills the process, so
// without this a single malformed answer from a service would end every
// generation in flight along with it.
func recoverPanics(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		defer func() {
			if problem := recover(); problem != nil {
				log.Printf("[error] panic while handling an update: %v\n%s", problem, debug.Stack())
			}
		}()

		next(ctx, b, update)
	}
}

func New(ctx context.Context) *bot.Bot {
	opts := []bot.Option{
		bot.WithDefaultHandler(getHandler()),
		bot.WithMiddlewares(recoverPanics),
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
