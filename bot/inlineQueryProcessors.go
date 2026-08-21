package bot

import (
	"context"
	"fmt"
	"log"
	"telebot/model"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
)

// buildInlineImageResults offers the pictures from the user's buffer for
// animation, most recently used first. Whatever is typed into the inline query
// becomes the prompt, so the same picture can be animated differently every
// time; an empty query leaves the prompt to the animator.
//
// The pictures are offered as articles rather than as cached photos on purpose:
// clients draw a photo result as a bare thumbnail and show neither its title nor
// its description, which left these rows wordless next to the other commands. An
// article shows both, and gets its preview from thumbnailURL — which is empty
// when no public address is configured, leaving the row without a picture but
// still readable.
func buildInlineImageResults(userId int64, prompt string) []models.InlineQueryResult {
	images, err := model.GetInlineImages(userId, model.InlineImageLimit)
	if err != nil {
		log.Println("[error] error getting inline images", err)
		return nil
	}

	promptText := "оживлю как есть, без описания"
	if prompt != "" {
		promptText = "оживлю так: " + prompt
	}

	results := make([]models.InlineQueryResult, 0, len(images))
	for i, image := range images {
		key := uuid.NewString()
		queries.setValue(key, callbackQueryData{
			kind:    callbackQueryImageVideo,
			query:   prompt,
			fileID:  image.FileID,
			ownerID: userId,
		})

		btn := models.InlineKeyboardButton{Text: "Анимируем!!!", CallbackData: key}
		line := []models.InlineKeyboardButton{btn}
		kbd := [][]models.InlineKeyboardButton{line}

		title := fmt.Sprintf("Анимировать мою картинку №%d", i+1)
		description := fmt.Sprintf("Добавлена %s — %s",
			image.UpdatedAt.UTC().Format("02.01 15:04"), promptText)

		results = append(results, &models.InlineQueryResultArticle{
			ID:                  key,
			Title:               title,
			Description:         description,
			ThumbnailURL:        thumbnailURL(image.Token),
			InputMessageContent: models.InputTextMessageContent{MessageText: title},
			ReplyMarkup:         models.InlineKeyboardMarkup{InlineKeyboard: kbd},
		})
	}

	return results
}

func processInlineQuery(ctx context.Context, b *bot.Bot, update *models.Update) {
	var description string
	if update.InlineQuery.Query != "" {
		description = update.InlineQuery.Query
	} else {
		description = "Кота"
	}

	userId := update.InlineQuery.From.ID

	key := uuid.NewString()
	queries.setValue(key, callbackQueryData{kind: callbackQueryImage, query: description})

	btn := models.InlineKeyboardButton{Text: "Стартуем!!!", CallbackData: key}
	line := []models.InlineKeyboardButton{btn}
	kbd := [][]models.InlineKeyboardButton{line}

	videoKey := uuid.NewString()
	queries.setValue(videoKey, callbackQueryData{kind: callbackQueryTextVideo, query: description})

	videoBtn := models.InlineKeyboardButton{Text: "Анимируем!!!", CallbackData: videoKey}
	videoLine := []models.InlineKeyboardButton{videoBtn}
	videoKbd := [][]models.InlineKeyboardButton{videoLine}

	sizeText := buildSizeText(userId)
	depthText := buildDepthText(userId)

	results := []models.InlineQueryResult{
		&models.InlineQueryResultArticle{
			ID:                  "draw",
			Title:               "Что рисуем?",
			Description:         description,
			InputMessageContent: models.InputTextMessageContent{MessageText: description},
			ReplyMarkup:         models.InlineKeyboardMarkup{InlineKeyboard: kbd},
		},
		&models.InlineQueryResultArticle{
			ID:                  "animate",
			Title:               "Что анимируем?",
			Description:         description,
			InputMessageContent: models.InputTextMessageContent{MessageText: description},
			ReplyMarkup:         models.InlineKeyboardMarkup{InlineKeyboard: videoKbd},
		},
		&models.InlineQueryResultArticle{
			ID:                  "size",
			Title:               "Размер",
			Description:         "Каков твой дружок",
			InputMessageContent: models.InputTextMessageContent{MessageText: sizeText},
		},
		&models.InlineQueryResultArticle{
			ID:                  "depth",
			Title:               "Глубина Матильды",
			Description:         "Состояние подруженьки",
			InputMessageContent: models.InputTextMessageContent{MessageText: depthText},
		},
	}
	results = append(results, buildInlineImageResults(userId, update.InlineQuery.Query)...)

	_, err := b.AnswerInlineQuery(ctx, &bot.AnswerInlineQueryParams{
		InlineQueryID: update.InlineQuery.ID,
		IsPersonal:    true,
		CacheTime:     1,
		Button: &models.InlineQueryResultsButton{
			Text:           "Закинуть картинку для анимации",
			StartParameter: inlineImageUploadStartParam,
		},
		Results: results,
	})
	if err != nil {
		log.Println(err)
	}

	// Every keystroke answers a fresh inline query and leaves a callback entry
	// per result behind, so this is the busiest place to sweep the map from.
	queries.deleteOldValues()
}
