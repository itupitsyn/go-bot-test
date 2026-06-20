package utils

import (
	"context"
	"fmt"
	"math/rand/v2"
	"telebot/model"
	"time"

	"log"
	"strings"

	"github.com/go-telegram/bot"

	"github.com/go-telegram/bot/models"
)

func ProcessSendMessageError(err error, chatId int64) {
	if err != nil {
		log.Printf("[error] couldn't send message to %d\n", chatId)
		log.Println(err)
	}
}

func GetAlternativeName(user *models.User) string {
	var nameParts []string

	if user.FirstName != "" {
		nameParts = append(nameParts, user.FirstName)
	}
	if user.LastName != "" {
		nameParts = append(nameParts, user.LastName)
	}

	return strings.Join(nameParts, "")
}

func GetAnyName(user *models.User) string {
	if user.Username != "" {
		return user.Username
	}

	return GetAlternativeName(user)
}

// SendPhrazes posts each phraze to the chat. If a non-zero replyTo message ID
// is passed, every phraze is sent as a reply to that message.
func SendPhrazes(ctx context.Context, b *bot.Bot, chat *model.Chat, phrazes []model.Phraze, replyTo ...int) {
	chatId := chat.ID

	var replyParams *models.ReplyParameters
	if len(replyTo) > 0 && replyTo[0] != 0 {
		replyParams = &models.ReplyParameters{MessageID: replyTo[0]}
	}

	for _, phraze := range phrazes {

		if phraze.IsWithSpoiler {
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          chatId,
				Text:            fmt.Sprintf("<tg-spoiler>%s</tg-spoiler>", phraze.Value),
				ParseMode:       models.ParseModeHTML,
				ReplyParameters: replyParams,
			})
			ProcessSendMessageError(err, chatId)
		} else {
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          chat.ID,
				Text:            phraze.Value,
				ReplyParameters: replyParams,
			})
			ProcessSendMessageError(err, chatId)
		}

		duration := rand.IntN(5) + 1

		time.Sleep(time.Duration(duration) * time.Second)
	}
}

func TrimPrefixIgnoreCase(s, prefix string) string {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):]
	}
	return s
}
