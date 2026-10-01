package bot

import (
	"context"
	"log"
	"telebot/model"
	"telebot/utils"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// What a command is recorded as. Nothing depends on these strings — they are
// there so the table can be read later and understood.
const (
	aiKindImage   = "image"
	aiKindEdit    = "edit"
	aiKindVideo   = "video"
	aiKindSticker = "sticker"
)

// aiLimitFallbackText is said when neither the person nor the admin panel has
// a wording of its own. Deliberately free of numbers: the quota is a private
// matter between the admin and that person, and announcing "20 per hour" in a
// group chat invites an argument about why someone else has more.
const aiLimitFallbackText = "Слишком часто. Нейронки не резиновые, попробуй позже."

// aiLimitRefusalText picks what to answer: the person's own wording, otherwise
// the general one from the admin panel, otherwise ours.
func aiLimitRefusalText(limit *model.AiLimit) string {
	if limit != nil && limit.Message != "" {
		return limit.Message
	}

	settings, err := model.GetAiLimitSettings()
	if err != nil {
		log.Println("[error] error getting ai limit settings")
		log.Println(err)
	}
	if settings != nil && settings.Message != "" {
		return settings.Message
	}

	return aiLimitFallbackText
}

// chargeAiLimit checks the person's quota and spends `units` of it.
//
// Spending happens BEFORE the work, not after, and on purpose: a sticker pack
// runs for minutes, and a quota charged at the end would let somebody start ten
// packs while the first one is still going. A pack that comes out short gives
// the unused units back through the returned row.
//
// People without a quota are not recorded at all. Almost nobody has one, and a
// row per picture for the whole chat would be a log nobody asked for.
//
// Every failure here lets the request through. The limit is housekeeping; the
// bot going silent because the database hiccuped would be worse than one
// picture over the quota.
func chargeAiLimit(userID int64, kind string, units int) (*model.AiUsage, string, bool) {
	limit, err := model.GetAiLimit(userID)
	if err != nil {
		log.Println("[error] error getting ai limit")
		log.Println(err)
		return nil, "", true
	}
	if !limit.IsActive() {
		return nil, "", true
	}

	used, err := model.CountAiUsage(userID, time.Now().Add(-model.AiLimitWindow(limit.Period)))
	if err != nil {
		log.Println("[error] error counting ai usage")
		log.Println(err)
		return nil, "", true
	}

	if used+units > limit.Amount {
		return nil, aiLimitRefusalText(limit), false
	}

	usage, err := model.AddAiUsage(userID, kind, units)
	if err != nil {
		// Записать не вышло — работу всё равно делаем. Иначе сбой базы
		// превращается в отказ всем, у кого есть лимит.
		log.Println("[error] error adding ai usage")
		log.Println(err)
	}

	return usage, "", true
}

// replyIfOverAiLimit answers that the person has run out of quota and returns
// true if the command should go no further. The returned row lets the caller
// give unused units back; for everything but a sticker pack it is ignored.
func replyIfOverAiLimit(ctx context.Context, b *bot.Bot, message *models.Message,
	kind string, units int) (*model.AiUsage, bool) {
	if message == nil || message.From == nil {
		return nil, false
	}

	usage, text, ok := chargeAiLimit(message.From.ID, kind, units)
	if ok {
		return usage, false
	}

	log.Println("AI command refused: over the limit,", utils.GetAnyName(message.From))

	chatID := message.Chat.ID
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatID,
		Text:            text,
		ReplyParameters: &models.ReplyParameters{MessageID: message.ID},
	})
	utils.ProcessSendMessageError(err, chatID)

	return nil, true
}

// refuseAiCommand is the single-picture case: charge one unit, answer if there
// is none left. Written as its own function so the chain in bot.go reads the
// same way as the maintenance check next to it.
func refuseAiCommand(ctx context.Context, b *bot.Bot, message *models.Message, kind string) bool {
	_, refused := replyIfOverAiLimit(ctx, b, message, kind, 1)

	return refused
}
