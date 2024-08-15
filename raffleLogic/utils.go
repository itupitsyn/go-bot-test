package raffleLogic

import (
	"errors"
	"fmt"
	"math/rand"
	"telebot/model"

	tgbotapi "github.com/matterbridge/telegram-bot-api/v6"
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
	msg := tgbotapi.NewMessage(chatId, fmt.Sprintf("%s выигрывает %s!!", winnerName, prizeName))
	msg.ParseMode = "HTML"
	_, err = bot.Send(msg)
	return err
}

func GetRandomPhrazeByKey(key string) string {
	phrazes, err := model.GetPharzesByKey(key)
	if err != nil {
		fmt.Println(err)
	}
	if phrazes != nil {
		count := len(*phrazes)
		if count < 1 {
			return ""
		}
		phrazeIdx := rand.Intn(count)
		return (*phrazes)[phrazeIdx].Value
	}
	return ""
}
