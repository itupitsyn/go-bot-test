package bot

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"telebot/raffleLogic"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var botName string = ""

func handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.InlineQuery != nil {
		processInlineQuery(ctx, b, update)
	} else if update.CallbackQuery != nil {
		processCallbackQuery(ctx, b, update)
	} else if update.Message != nil {
		log.Println("Received message from", update.Message.From.Username)

		msgTextLower := strings.ToLower(update.Message.Text)
		if strings.HasPrefix(msgTextLower, "нарисуй ") || strings.HasPrefix(msgTextLower, "draw ") {
			log.Println("Image generation requested by", update.Message.From.Username)
			processImageGeneration(ctx, b, update)
		} else if strings.HasPrefix(msgTextLower, "/ai_help") || strings.HasPrefix(msgTextLower, "/ai_help@"+botName) {
			log.Println("AI help requested by", update.Message.From.Username)
			processAIHelp(ctx, b, update)
		}

		msgType := update.Message.Chat.Type
		if msgType != "group" && msgType != "supergroup" {
			return
		}

		if !raffleLogic.IsNoReturnPoint() {
			log.Println("Running processParticipation")
			processParticipation(update)
		}

		if msgTextLower == "/stats" || strings.HasPrefix(msgTextLower, "/stats@"+botName) {
			log.Println("Stats requested by", update.Message.From.Username)
			processStats(ctx, b, update, false)
		} else if msgTextLower == "/stats_full" || strings.HasPrefix(msgTextLower, "/stats_full@"+botName) {
			log.Println("Full stats requested by", update.Message.From.Username)
			processStats(ctx, b, update, true)
		} else if strings.HasPrefix(msgTextLower, "сегодня ") || strings.HasPrefix(msgTextLower, "завтра ") {
			log.Println("Prize requested by", update.Message.From.Username)
			processPrize(ctx, b, update)
		} else if msgTextLower == "/prize" || strings.HasPrefix(msgTextLower, "/prize@"+botName) {
			log.Println("Prize info requested by", update.Message.From.Username)
			processPrizeInfo(ctx, b, update.Message.Chat.ID)
		} else if strings.HasPrefix(msgTextLower, "/set_admin") || strings.HasPrefix(msgTextLower, "/set_admin@"+botName) {
			log.Println("Setting admin requested by", update.Message.From.Username)
			processSetAdmin(ctx, b, update)
		} else if strings.HasPrefix(msgTextLower, "/unset_admin") || strings.HasPrefix(msgTextLower, "/unset_admin@"+botName) {
			log.Println("Unsetting admin requested by", update.Message.From.Username)
			processUnsetAdmin(ctx, b, update)
		} else if strings.HasPrefix(msgTextLower, "/admins") || strings.HasPrefix(msgTextLower, "/admins@"+botName) {
			log.Println("Admins requested by", update.Message.From.Username)
			processAdmins(ctx, b, update)
		}

	}
}

func Listen() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	opts := []bot.Option{
		bot.WithDefaultHandler(handler),
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

	b.Start(ctx)
}
