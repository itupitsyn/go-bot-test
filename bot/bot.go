package bot

import (
	"fmt"
	"os"
	"strings"
	"telebot/model"
	"telebot/raffleLogic"
	"time"
	"unicode/utf8"

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
		if update.Message == nil {
			continue
		}

		msgType := update.Message.Chat.Type
		if msgType != "group" && msgType != "supergroup" {
			continue
		}

		if !raffleLogic.IsNoReturnPoint() {
			usr := model.User{
				ID:   update.Message.From.ID,
				Name: update.Message.From.UserName,
			}
			usr.Save()

			raffle := model.Raffle{
				ChatID:       update.Message.Chat.ID,
				Date:         datatypes.Date(time.Now()),
				Participants: []model.User{},
			}
			raffle.Save()

			participants := model.Raffle{
				ChatID: update.Message.Chat.ID,
				Date:   datatypes.Date(time.Now()),
				Participants: []model.User{
					usr,
				},
			}
			participants.Save()
		}

		if update.Message.Text == "/stats" || strings.HasPrefix(update.Message.Text, "/stats@"+bot.Self.UserName) {
			stats := model.GetStats(update.Message.Chat.ID)
			var msgText string
			if len(*stats) == 0 {
				msgText = "There is no stats yet"
			} else {
				maxNameLen := utf8.RuneCountInString("winner")
				maxCountsLen := utf8.RuneCountInString("wins")
				for _, current := range *stats {
					currLen := utf8.RuneCountInString(current.Name)
					if maxNameLen < currLen {
						maxNameLen = currLen
					}
					currLen = utf8.RuneCountInString(fmt.Sprint(current.Count))
					if maxCountsLen < currLen {
						maxCountsLen = currLen
					}
				}
				msgText = "<code>"
				msgText += fmt.Sprintf("%-*s %*s\n", maxNameLen, "winner", maxCountsLen, "wins")
				for _, current := range *stats {
					msgText += fmt.Sprintf("%-*s %*d\n", maxNameLen, current.Name, maxCountsLen, current.Count)
				}
				msgText += "</code>"
			}

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)
			msg.ParseMode = "HTML"
			_, err := bot.Send(msg)
			if err != nil {
				fmt.Println(err)
			}
		}
	}
}
