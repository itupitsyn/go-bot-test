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

// maintenanceText is the answer to an AI command during maintenance.
//
// Time is written in UTC like everything else in the bot: the raffle in /help
// is announced in UTC too, and there is no point keeping two time zones in
// mind.
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

// activeMaintenanceText says whether maintenance is going on right now and what
// to answer then. If the database can't be queried, we assume there is no
// maintenance: better to try the service than to switch the neural nets off
// over a glitch.
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

// replyIfMaintenance answers an AI command saying the neural nets are under
// maintenance. It returns true if that is the case and the command should go no
// further.
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
