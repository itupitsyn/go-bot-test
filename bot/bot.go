package bot

import (
	"context"
	"log"
	"os"
	"os/signal"
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

type aiGenerationProcessorChanel struct {
	update        *models.Update
	mainMessageId int
}

func getHandler(imgChannel chan *aiGenerationProcessorChanel, videoChannel chan *aiGenerationProcessorChanel) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {

		sendWaitInlineQueryMessage := func(msgId string) error {
			_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
				InlineMessageID: msgId,
				Text:            "ОЖИДАЕМ!!!",
			})
			return err
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
			log.Println("Image generation requested by", utils.GetAnyName(&update.CallbackQuery.From))
			sendWaitInlineQueryMessage(update.CallbackQuery.InlineMessageID)
			imgChannel <- &aiGenerationProcessorChanel{
				update: update,
			}
		} else if update.Message != nil {
			chatId := update.Message.Chat.ID
			saveUser(update.Message.From)
			userName := utils.GetAnyName(update.Message.From)
			log.Println("Received message from", userName)

			msgTextLower := strings.ToLower(update.Message.Text)
			if msgTextLower == "" && update.Message.Photo != nil && len(update.Message.Photo) > 0 {
				msgTextLower = strings.ToLower(update.Message.Caption)
			}

			if strings.HasPrefix(msgTextLower, "нарисуй ") || strings.HasPrefix(msgTextLower, "draw ") {
				log.Println("Image generation requested by", userName)
				mainMessageId := sendWaitMessage(chatId, update.Message.ID)
				imgChannel <- &aiGenerationProcessorChanel{
					update:        update,
					mainMessageId: mainMessageId,
				}
			} else if strings.HasPrefix(msgTextLower, "анимируй") || strings.HasPrefix(msgTextLower, "animate") {
				log.Println("Video generation requested by", userName)
				mainMessageId := sendWaitMessage(chatId, update.Message.ID)
				videoChannel <- &aiGenerationProcessorChanel{
					update:        update,
					mainMessageId: mainMessageId,
				}
			} else if strings.HasPrefix(msgTextLower, "/ai_help") || strings.HasPrefix(msgTextLower, "/ai_help@"+botName) {
				log.Println("AI help requested by", userName)
				processAIHelp(ctx, b, update)
			} else if strings.HasPrefix(msgTextLower, "/help") || strings.HasPrefix(msgTextLower, "/help@"+botName) {
				log.Println("Help requested by", userName)
				processHelp(ctx, b, update)
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

func Listen() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	imgChannel := make(chan *aiGenerationProcessorChanel)
	videoChannel := make(chan *aiGenerationProcessorChanel)

	opts := []bot.Option{
		bot.WithDefaultHandler(getHandler(imgChannel, videoChannel)),
		bot.WithAllowedUpdates([]string{"callback_query", "message", "inline_query"}),
	}

	b, err := bot.New(os.Getenv("TELEGRAM_APITOKEN"), opts...)
	if err != nil {
		panic(err)
	}

	go processImgAiQueue(imgChannel, ctx, b)
	go processVideoAiQueue(videoChannel, ctx, b)

	self, err := b.GetMe(ctx)
	if err != nil {
		panic(err)
	}
	botName = self.Username

	b.Start(ctx)
}
