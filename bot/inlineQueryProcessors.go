package bot

import (
	"context"
	"log"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
)

func processInlineQuery(ctx context.Context, b *bot.Bot, update *models.Update) {
	var description string
	if update.InlineQuery.Query != "" {
		description = update.InlineQuery.Query
	} else {
		description = "Кота"
	}

	key := uuid.NewString()
	queries.setValue(key, description)

	btn := models.InlineKeyboardButton{Text: "Стартуем!!!", CallbackData: key}
	line := []models.InlineKeyboardButton{btn}
	kbd := [][]models.InlineKeyboardButton{line}

	_, err := b.AnswerInlineQuery(ctx, &bot.AnswerInlineQueryParams{
		InlineQueryID: update.InlineQuery.ID,
		IsPersonal:    true,
		CacheTime:     1,
		Results: []models.InlineQueryResult{
			&models.InlineQueryResultArticle{
				ID:                  "draw",
				Title:               "Что рисуем?",
				Description:         description,
				InputMessageContent: models.InputTextMessageContent{MessageText: description},
				ReplyMarkup:         models.InlineKeyboardMarkup{InlineKeyboard: kbd},
			},
		},
	})
	if err != nil {
		log.Println(err)
	}
}
