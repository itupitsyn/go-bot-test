package bot

import (
	"os"
	"telebot/model"
	"telebot/raffleLogic"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/datatypes"
)

func Listen() {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_APITOKEN"))
	if err != nil {
		panic(err)
	}

	// bot.Debug = true

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30

	// Start polling Telegram for updates.
	updates := bot.GetUpdatesChan(updateConfig)

	for update := range updates {
		if update.Message == nil || raffleLogic.IsNoReturnPoint() {
			continue
		}

		usr := model.User{
			ID:   update.Message.From.ID,
			Name: update.Message.From.UserName,
		}
		usr.Save()

		msgType := update.Message.Chat.Type
		if msgType != "group" && msgType != "supergroup" {
			continue
		}

		raffle := model.Raffle{
			ChatID:       update.Message.Chat.ID,
			Date:         datatypes.Date(update.Message.Time()),
			Participants: []model.User{},
		}
		raffle.Save()

		participants := model.Raffle{
			ChatID: update.Message.Chat.ID,
			Date:   datatypes.Date(update.Message.Time()),
			Participants: []model.User{
				usr,
			},
		}
		participants.Save()
	}
}
