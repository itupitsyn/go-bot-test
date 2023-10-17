package raffleLogic

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"telebot/model"
	"telebot/utils"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var noReturnPoint = time.Date(1970, 1, 1, 12, 0, 0, 0, time.UTC)

var ticker = time.NewTicker(60 * time.Second)
var quit = make(chan struct{})

func Listen() {
	for {
		select {
		case <-ticker.C:
			if IsNoReturnPoint() {
				runRaffles()
			}
		case <-quit:
			ticker.Stop()
			return
		}
	}
}

func IsNoReturnPoint() bool {
	now := time.Now().In(time.UTC)
	checkPoint := time.Date(1970, 1, 1, now.Hour(), now.Minute(), now.Second(), 0, now.Location())

	return checkPoint.After(noReturnPoint)
}

func runRaffles() ([]model.Raffle, error) {

	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_APITOKEN"))
	if err != nil {
		panic(err)
	}

	var raffle = model.Raffle{}

	raffleDate := datatypes.Date(time.Now())
	raffles, err := raffle.GetRafflesByDate(raffleDate)

	for _, currentRaffle := range raffles {
		winner := runRaffle(&currentRaffle)
		if winner == nil {
			continue
		}
		go sendResultWithPrep(bot, currentRaffle.ChatID, raffleDate, winner.Name)
	}

	return raffles, err
}

func runRaffle(currentRaffle *model.Raffle) *model.User {
	participantsCount := len(currentRaffle.Participants)
	if currentRaffle.WinnerID != nil || participantsCount < 2 {
		return nil
	}
	winnerIdx := rand.Intn(participantsCount)
	winner := currentRaffle.Participants[winnerIdx]
	currentRaffle.WinnerID = &winner.ID
	currentRaffle.Save()
	return &winner
}

func sendResultWithPrep(bot *tgbotapi.BotAPI, chatId int64, date datatypes.Date, winnerName string) {
	_, err := bot.Send(tgbotapi.NewMessage(chatId, "Пора!"))
	utils.ProcessSendMessageError(err, chatId)
	time.Sleep(2 * time.Second)
	_, err = bot.Send(tgbotapi.NewMessage(chatId, "ПОРАААА!!!"))
	utils.ProcessSendMessageError(err, chatId)
	time.Sleep(2 * time.Second)
	msg := tgbotapi.NewMessage(chatId, "<tg-spoiler>КРУТИМ, БЛЯДЬ!!!</tg-spoiler>")
	msg.ParseMode = "HTML"
	_, err = bot.Send(msg)
	utils.ProcessSendMessageError(err, chatId)
	time.Sleep(5 * time.Second)
	err = SendResult(bot, chatId, date, winnerName)
	utils.ProcessSendMessageError(err, chatId)
}

func SendResult(bot *tgbotapi.BotAPI, chatId int64, date datatypes.Date, winnerName string) error {
	prize, err := model.GetPrizeByDate(date, chatId)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	prizeName := "обыденное нихуя"
	if prize.Name != "" {
		prizeName = prize.Name
	}
	msg := tgbotapi.NewMessage(chatId, fmt.Sprintf("@%s выигрывает %s!!", winnerName, prizeName))
	_, err = bot.Send(msg)
	return err
}
