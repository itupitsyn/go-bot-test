package bot

import (
	"context"
	"fmt"
	"log"
	"telebot/model"
	"telebot/utils"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// maintenanceText — ответ на AI-команду во время профилактики.
//
// Время пишем в UTC, как и всё остальное в боте: розыгрыш в /help тоже
// объявлен по UTC, и держать в голове два пояса незачем.
func maintenanceText(maintenance *model.AiMaintenance, now time.Time) string {
	const prefix = "Нейронки на профилактике"

	if maintenance.EndsAt == nil {
		return prefix + ", скоро вернёмся"
	}

	endsAt := maintenance.EndsAt.UTC()
	today := now.UTC()
	day := endsAt.Format("02.01")
	switch {
	case sameDate(endsAt, today):
		day = "сегодня"
	case sameDate(endsAt, today.AddDate(0, 0, 1)):
		day = "завтра"
	}

	return fmt.Sprintf("%s, вернёмся %s в %s UTC", prefix, day, endsAt.Format("15:04"))
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// activeMaintenanceText говорит, идёт ли сейчас профилактика, и что тогда
// ответить. Если базу спросить не вышло, считаем, что профилактики нет:
// лучше попробовать сходить в сервис, чем выключить нейронки из-за сбоя.
func activeMaintenanceText() (string, bool) {
	maintenance, err := model.GetAiMaintenance()
	if err != nil {
		log.Println("[error] error getting ai maintenance")
		log.Println(err)
		return "", false
	}

	now := time.Now()
	if !maintenance.IsActive(now) {
		return "", false
	}

	return maintenanceText(maintenance, now), true
}

// replyIfMaintenance отвечает на AI-команду, что нейронки на профилактике.
// Вернёт true, если так и есть и команду дальше вести не нужно.
func replyIfMaintenance(ctx context.Context, b *bot.Bot, message *models.Message) bool {
	text, ok := activeMaintenanceText()
	if !ok {
		return false
	}

	log.Println("AI command refused: maintenance")

	chatId := message.Chat.ID
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatId,
		Text:            text,
		ReplyParameters: &models.ReplyParameters{MessageID: message.ID},
	})
	utils.ProcessSendMessageError(err, chatId)

	return true
}
