package raffleLogic

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"os"
	"telebot/model"
	"telebot/utils"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/datatypes"
)

var ticker = time.NewTicker(60 * time.Second)
var quit = make(chan struct{})

var defaultPrizeName = "обыденное нихуя"

func Listen() {
	for {
		select {
		case <-ticker.C:
			if IsNoReturnPoint() {
				runRaffles()
			}
		case <-quit:
			ticker.Stop()
			log.Println("Raffle logic stopped")
			return
		}
	}
}

func IsNoReturnPoint() bool {
	now := time.Now().UTC()
	return now.Hour() >= 12
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
		var name string
		if winner.Name != "" {
			name = "@" + winner.Name
		} else {
			name = fmt.Sprintf("<a href=\"tg://user?id=%d\">%s</a>", winner.ID, winner.AlternativeName)
		}

		go sendResultWithPrep(bot, currentRaffle.ChatID, raffleDate, name)
	}

	return raffles, err
}

func runRaffle(currentRaffle *model.Raffle) *model.User {
	participantsCount := len(currentRaffle.Participants)
	if currentRaffle.WinnerID != nil || participantsCount < 2 {
		return nil
	}
	winnerIdx, err := rand.Int(rand.Reader, big.NewInt(int64(participantsCount)))
	if err != nil {
		panic(err)
	}
	winner := currentRaffle.Participants[winnerIdx.Int64()]
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
