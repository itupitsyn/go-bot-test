package bot

import (
	"fmt"
	"os"
	"strings"
	"telebot/model"
	"telebot/raffleLogic"
	"telebot/utils"
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
			processParticipation(update)
		}

		msgTextLower := strings.ToLower(update.Message.Text)
		if update.Message.Text == "/stats" || strings.HasPrefix(update.Message.Text, "/stats@"+bot.Self.UserName) {
			processStats(bot, update)
		} else if strings.HasPrefix(msgTextLower, "сегодня ") || strings.HasPrefix(msgTextLower, "завтра ") {
			processPrize(bot, update)
		} else if update.Message.Text == "/prize" || strings.HasPrefix(update.Message.Text, "/prize@"+bot.Self.UserName) {
			processPrizeInfo(bot, update.Message.Chat.ID)
		}
	}
}

func processStats(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	stats := model.GetStats(update.Message.Chat.ID)

	var msgText string
	if len(*stats) == 0 {
		msgText = "Статистики еще нет. Здеся"
	} else {
		maxNameLen := 17
		maxCountsLen := 4
		msgText = "<code>"
		msgText += fmt.Sprintf("%-*s %*s\n", maxNameLen, "winner", maxCountsLen, "wins")
		for _, current := range *stats {
			var currentName string
			if current.Name != "" {
				currentName = current.Name
			} else {
				currentName = current.Alternativename
			}
			if utf8.RuneCountInString(currentName) > maxNameLen {
				currentName = currentName[:maxNameLen-2] + ".."
			}
			msgText += fmt.Sprintf("%-*s %*d\n", maxNameLen, currentName, maxCountsLen, current.Count)
		}
		msgText += "</code>"
	}

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)
	msg.ParseMode = "HTML"
	_, err := bot.Send(msg)
	utils.ProcessSendMessageError(err, update.Message.Chat.ID)
}

func processParticipation(update tgbotapi.Update) {
	from := update.Message.From
	name := from.UserName
	var nameParts []string

	if from.FirstName != "" {
		nameParts = append(nameParts, from.FirstName)
	}
	if from.LastName != "" {
		nameParts = append(nameParts, from.LastName)
	}
	alternativeName := strings.Join(nameParts, "")

	usr := model.User{
		ID:            from.ID,
		Name:          name,
		AlternativeName: alternativeName,
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

func processPrize(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	chatId := update.Message.Chat.ID
	admin := model.Admin{
		ChatID: chatId,
		UserID: &update.Message.From.ID,
	}

	phraseParts := strings.Split(strings.ToLower(update.Message.Text), " ")
	if len(phraseParts) < 2 {
		return
	}

	if ok, _ := admin.IsAdmin(); !ok {
		phraze := raffleLogic.GetRandomPhrazeByKey(raffleLogic.WrongAdminKey)
		msg := tgbotapi.NewMessage(chatId, phraze)
		msg.ReplyToMessageID = update.Message.MessageID
		_, err := bot.Send(msg)
		utils.ProcessSendMessageError(err, chatId)

		return
	}

	var date datatypes.Date
	if phraseParts[0] == "сегодня" {
		if raffleLogic.IsNoReturnPoint() {
			msg := tgbotapi.NewMessage(chatId, "ПОЗДНО!")
			msg.ReplyToMessageID = update.Message.MessageID
			_, err := bot.Send(msg)
			utils.ProcessSendMessageError(err, update.Message.Chat.ID)
			return
		} else {
			date = datatypes.Date(time.Now().In(time.UTC))
		}
	} else {
		date = datatypes.Date(time.Now().In(time.UTC).AddDate(0, 0, 1))
	}
	phraseParts = strings.Split(update.Message.Text, " ")
	newPrize := strings.Join(phraseParts[1:], " ")

	model.DeletePrizeByDate(date, chatId)
	prize := model.Prize{
		Name:   newPrize,
		ChatID: chatId,
		Date:   date,
	}
	prize.Save()

	phraze := raffleLogic.GetRandomPhrazeByKey(raffleLogic.AcceptPrizeKey)

	msg := tgbotapi.NewMessage(chatId, phraze)
	msg.ReplyToMessageID = update.Message.MessageID
	_, err := bot.Send(msg)
	utils.ProcessSendMessageError(err, chatId)
}

func processPrizeInfo(bot *tgbotapi.BotAPI, chatId int64) {
	year, month, day := time.Now().In(time.UTC).Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)

	dates := []datatypes.Date{datatypes.Date(today), datatypes.Date(today.AddDate(0, 0, 1))}
	prizes, _ := model.GetPrizesByDate(dates, chatId)

	prizeToday := raffleLogic.GetPrizeName(nil)
	prizeTomorrow := raffleLogic.GetPrizeName(nil)
	for _, prize := range *prizes {
		if prize.Date == dates[0] {
			prizeToday = raffleLogic.GetPrizeName(&prize)
		} else {
			prizeTomorrow = raffleLogic.GetPrizeName(&prize)
		}
	}

	msg := tgbotapi.NewMessage(chatId, fmt.Sprintf("Сегодня — %s \nЗавтра — %s", prizeToday, prizeTomorrow))
	_, err := bot.Send(msg)
	utils.ProcessSendMessageError(err, chatId)
}
