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
	"gorm.io/datatypes"
)

var ticker = time.NewTicker(60 * time.Second)
var quit = make(chan struct{})

var defaultPrizeNameUncensored = "обыденное нихуя"
var defaultPrizeName = "обыденное ничего"

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
	chat, err := model.GetChatById(chatId)
	if err != nil {
		log.Fatal("error getting chat while sending result", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	phrazes := GetRandomPhrazeByKey(RaffleProvidingKey, chat.IsUncensored)

	if len(phrazes) == 0 {
		if chat.IsUncensored {
			phrazes = []model.Phraze{{Value: "Пора!", IsWithSpoiler: false}, {Value: "ПОРАААА!!!", IsWithSpoiler: false}, {Value: "КРУТИМ, БЛЯДЬ!!!", IsWithSpoiler: true}}
		} else {
			phrazes = []model.Phraze{{Value: "Пора!", IsWithSpoiler: false}, {Value: "ПОРАААА!!!", IsWithSpoiler: false}, {Value: "КРУТИМ!!!", IsWithSpoiler: true}}
		}
	}

	utils.SendPhrazes(ctx, b, chat, phrazes)

	err = SendResult(ctx, b, chat, date, winnerName)
	utils.ProcessSendMessageError(err, chatId)
}
