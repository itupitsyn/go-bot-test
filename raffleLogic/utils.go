package raffleLogic

import (
	"context"
	"errors"
	"fmt"
	"log"

	"math/rand"
	"telebot/model"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var AcceptPrizeKey = "accept_prize"
var WrongAdminKey = "wrong_admin"
var RaffleProvidingKey = "raffle_providing"

func GetPrizeName(prize *model.Prize, chat *model.Chat) string {
	var prizeName string
	if chat.IsUncensored {
		prizeName = defaultPrizeNameUncensored
	} else {
		prizeName = defaultPrizeName
	}

	if prize != nil && prize.Name != "" {
		prizeName = prize.Name
	}
	return prizeName
}

func SendResult(ctx context.Context, b *bot.Bot, chat *model.Chat, date datatypes.Date, winnerName string) error {
	chatId := chat.ID
	prize, err := model.GetPrizeByDate(date, chatId)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	prizeName := GetPrizeName(prize, chat)

	msgText := fmt.Sprintf("%s выигрывает %s!!", winnerName, prizeName)
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatId,
		Text:      msgText,
		ParseMode: models.ParseModeHTML,
	})

	return err
}

type IntWithNull struct {
	Value  int
	IsNull bool
}

func uniqueInts(arr []IntWithNull) []IntWithNull {
	seen := make(map[IntWithNull]bool)
	result := make([]IntWithNull, 0)

	for _, item := range arr {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

func GetRandomPhrazeByKey(key string, isUncensord bool) []model.Phraze {
	phrazes, err := model.GetPharzesByKey(key, isUncensord)
	if err != nil {
		log.Println(err)
	}

	res := []model.Phraze{}

	if phrazes == nil || len(*phrazes) < 1 {
		return res
	}

	groupValues := []IntWithNull{}

	for _, val := range *phrazes {
		if val.Group == nil {
			groupValues = append(groupValues, IntWithNull{IsNull: true})
		} else {
			groupValues = append(groupValues, IntWithNull{Value: *val.Group})
		}
	}

	groups := uniqueInts(groupValues)

	groupIdx := rand.Intn(len(groups))

	currGroup := groups[groupIdx]
	if currGroup.IsNull {
		data := []model.Phraze{}

		for _, val := range *phrazes {
			if val.Group == nil {
				data = append(data, val)
			}
		}

		phrazeIdx := rand.Intn(len(data))
		res = append(res, data[phrazeIdx])
	} else {
		for _, val := range *phrazes {
			if val.Group != nil && *val.Group == currGroup.Value {
				res = append(res, val)
			}
		}
	}

	return res
}
