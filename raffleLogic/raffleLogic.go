package raffleLogic

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"telebot/model"
	"telebot/utils"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
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
	b, err := bot.New(os.Getenv("TELEGRAM_APITOKEN"))
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

		go sendResultWithPrep(b, currentRaffle.ChatID, raffleDate, name)
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

func sendResultWithPrep(b *bot.Bot, chatId int64, date datatypes.Date, winnerName string) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatId,
		Text:   "Пора!",
	})
	utils.ProcessSendMessageError(err, chatId)

	time.Sleep(2 * time.Second)
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatId,
		Text:   "ПОРАААА!!!",
	})

	utils.ProcessSendMessageError(err, chatId)
	time.Sleep(2 * time.Second)
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatId,
		Text:      "<tg-spoiler>КРУТИМ, БЛЯДЬ!!!</tg-spoiler>",
		ParseMode: models.ParseModeHTML,
	})
	utils.ProcessSendMessageError(err, chatId)

	time.Sleep(5 * time.Second)
	err = SendResult(ctx, b, chatId, date, winnerName)
	utils.ProcessSendMessageError(err, chatId)
}
