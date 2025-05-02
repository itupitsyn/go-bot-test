package bot

import (
	"context"
	"fmt"
	"log"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func processInlineQuery(ctx context.Context, b *bot.Bot, update *models.Update) {
	var description string
	if update.InlineQuery.Query != "" {
		description = update.InlineQuery.Query
	} else {
		description = "Кот"
	}

	btn := models.InlineKeyboardButton{Text: "Стартуем!!!", CallbackData: description}
	line := []models.InlineKeyboardButton{btn}
	kbd := [][]models.InlineKeyboardButton{line}

	_, err := b.AnswerInlineQuery(ctx, &bot.AnswerInlineQueryParams{
		InlineQueryID: update.InlineQuery.ID,
		IsPersonal:    true,
		Results: []models.InlineQueryResult{
			&models.InlineQueryResultArticle{
				ID:                  "draw",
				Title:               "Нарисовать",
				Description:         description,
				InputMessageContent: models.InputTextMessageContent{MessageText: fmt.Sprintf("Нарисовать %s", description)},
				ReplyMarkup:         models.InlineKeyboardMarkup{InlineKeyboard: kbd},
			},
		},
	})
	if err != nil {
		log.Println(err)
	}
}
