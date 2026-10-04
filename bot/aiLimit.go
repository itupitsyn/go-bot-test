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

// aiCharge is what a command took and how to give back what it did not use.
//
// Which pocket it came from the caller does not need to know: a sticker pack
// that came out short returns its unused units the same way whether they were
// free or bought.
type aiCharge struct {
	// usage is the row in the free quota, nil when nothing free was left.
	usage *model.AiUsage
	// free and paid are how much came out of each pocket. One command can
	// take from both: see chargeAiLimit.
	userID int64
	free   int
	paid   int
}

// setUnits rewrites what the command ended up costing.
//
// Лишнее возвращается СНАЧАЛА в платный карман и только потом в бесплатный.
// Деньги дороже квоты: неизрасходованная квота всё равно растворится вместе с
// окном, а купленное лежит, пока не потратят.
func (c *aiCharge) setUnits(used int) {
	if c == nil {
		return
	}

	back := c.free + c.paid - used
	if back < 0 {
		back = 0
	}

	backPaid := back
	if backPaid > c.paid {
		backPaid = c.paid
	}

	if c.usage != nil {
		if err := c.usage.SetUnits(c.free - (back - backPaid)); err != nil {
			log.Println("[error] error correcting the ai usage")
			log.Println(err)
		}
	}

	if backPaid > 0 {
		if err := model.AddAiCredit(c.userID, backPaid); err != nil {
			// Человек заплатил, работа не вся сделалась, а вернуть не вышло.
			// Кричим: это единственный след, по которому потом разобраться.
			log.Printf("[error] не вернул %d оплаченных генераций пользователю %d: %v\n",
				backPaid, c.userID, err)
		}
	}
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
func chargeAiLimit(userID int64, kind string, units int) (*aiCharge, string, bool) {
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

	// Бесплатное доедаем ДО копейки и платим только за остаток. Иначе человек
	// с девятью свободными генерациями и набором на десять заплатил бы за все
	// десять, хотя сверх лимита у него только одна.
	free := limit.Amount - used
	if free < 0 {
		free = 0
	}
	if free > units {
		free = units
	}

	// Проверка остатка и списание идут одним запросом, см. model.SpendAiCredit.
	// Ноль он пропускает сам, так что отдельной ветки «всё бесплатно» не надо.
	paid := units - free
	if paid > 0 {
		enough, err := model.SpendAiCredit(userID, paid)
		if err != nil {
			// По тому же правилу, что и выше: сбой базы не повод замолчать.
			// Дотуда, впрочем, почти не доходит — к этому месту база уже
			// дважды ответила, иначе мы вышли бы раньше.
			log.Println("[error] error spending ai credits")
			log.Println(err)

			return nil, "", true
		}
		if !enough {
			// Бесплатное НЕ списываем: команда не состоится целиком, и
			// отъедать за неё квоту не за что.
			return nil, aiLimitRefusalText(limit) + aiBuyOfferText(), false
		}
	}

	charge := &aiCharge{userID: userID, free: free, paid: paid}
	if free > 0 {
		usage, err := model.AddAiUsage(userID, kind, free)
		if err != nil {
			// Записать не вышло — работу всё равно делаем. Иначе сбой базы
			// превращается в отказ всем, у кого есть лимит.
			log.Println("[error] error adding ai usage")
			log.Println(err)
		}
		charge.usage = usage
	}

	return charge, "", true
}

// replyIfOverAiLimit answers that the person has run out of quota and returns
// true if the command should go no further. The returned row lets the caller
// give unused units back; for everything but a sticker pack it is ignored.
func replyIfOverAiLimit(ctx context.Context, b *bot.Bot, message *models.Message,
	kind string, units int) (*aiCharge, bool) {
	if message == nil || message.From == nil {
		return nil, false
	}

	charge, text, ok := chargeAiLimit(message.From.ID, kind, units)
	if ok {
		return charge, false
	}

	log.Println("AI command refused: over the limit,", utils.GetAnyName(message.From))

	chatID := message.Chat.ID
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatID,
		Text:            text,
		ReplyParameters: &models.ReplyParameters{MessageID: message.ID},
		// В личке кнопка зовёт счёт сюда же, в группе уводит в личку
		// диплинком: см. aiBuyKeyboard.
		ReplyMarkup: aiBuyKeyboard(message.Chat.Type == "private"),
	})
	utils.ProcessSendMessageError(err, chatID)

	return nil, true
}

// refuseAiCommand is the single-picture case: charge one unit, answer if there
// is none left.
//
// The charge comes back so that a generation which never happened can be given
// back. It matters most for bought units — taking somebody's money for a
// picture the service refused to draw is plain theft — but free units are
// returned the same way: the sticker pack has behaved like that since it was
// written, and two rules for the same thing would be worse than one.
func refuseAiCommand(ctx context.Context, b *bot.Bot, message *models.Message,
	kind string) (*aiCharge, bool) {
	return replyIfOverAiLimit(ctx, b, message, kind, 1)
}
