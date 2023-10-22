package raffleLogic

import (
	"errors"
	"fmt"
	"math/rand"
	"telebot/model"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var AcceptPrizeKey = "accept_prize"
var WrongAdminKey = "wrong_admin"

func GetPrizeName(prize *model.Prize) string {
	prizeName := defaultPrizeName

	if prize != nil && prize.Name != "" {
		prizeName = prize.Name
	}
	return prizeName
}

func SendResult(bot *tgbotapi.BotAPI, chatId int64, date datatypes.Date, winnerName string) error {
	prize, err := model.GetPrizeByDate(date, chatId)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	prizeName := GetPrizeName(prize)
	msg := tgbotapi.NewMessage(chatId, fmt.Sprintf("@%s выигрывает %s!!", winnerName, prizeName))
	_, err = bot.Send(msg)
	return err
}

func GetRandomPhrazeByKey(key string) string {
	phrazes, err := model.GetPharzesByKey(key)
	if err != nil {
		fmt.Println(err)
	}
	if phrazes != nil {
		phrazeIdx := rand.Intn(len(*phrazes))
		return (*phrazes)[phrazeIdx].Value
	}
	return ""
}
