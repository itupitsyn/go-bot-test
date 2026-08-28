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

// breakPoint looks for a place to cut the first limit runes: the last line
// break, failing that the last space, and only for a word longer than the
// limit itself — which has no separator to break on — the limit.
func breakPoint(runes []rune, limit int) int {
	for _, separator := range []rune{'\n', ' '} {
		for i := limit - 1; i >= 0; i-- {
			if runes[i] == separator {
				return i + 1
			}
		}
	}

	return limit
}

// SplitText cuts text into pieces of at most limit runes, keeping words whole
// where it can. Telegram refuses messages longer than 4096 characters, and a
// transcription of a long voice message goes past that easily.
func SplitText(text string, limit int) []string {
	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return []string{text}
	}

	var chunks []string
	appendChunk := func(chunk string) {
		if chunk = strings.TrimSpace(chunk); chunk != "" {
			chunks = append(chunks, chunk)
		}
	}

	for len(runes) > limit {
		cut := breakPoint(runes, limit)
		appendChunk(string(runes[:cut]))
		runes = runes[cut:]
	}
	appendChunk(string(runes))

	return chunks
}
