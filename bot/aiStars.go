package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"telebot/model"
	"telebot/utils"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Платные генерации поверх лимита.
//
// Лимит ставит админ отдельным людям, и до сих пор упереться в него значило
// просто подождать. Теперь у упёршегося есть второй выход: купить пачку
// генераций за Telegram Stars. Бесплатная квота при этом не меняется — см.
// model.AiCredit о том, почему это отдельный карман, а не прибавка к окну.
//
// Кто НЕ видит предложения: все, у кого лимита нет. Таких подавляющее
// большинство, они в отказ не упираются вовсе, и показывать им ценник не за
// что — они и так ничего не ждут.

// starsPackDefault и starsPriceDefault — сколько генераций и за сколько звёзд.
// Через окружение, потому что цену нащупывают живьём, а пересобирать бота ради
// одной цифры не хочется.
const (
	starsPackDefault  = 20
	starsPriceDefault = 5
)

// starsCurrency — валюта Telegram Stars. Единственная, у которой провайдерский
// токен пустой: деньги принимает сам Telegram.
const starsCurrency = "XTR"

// aiBuyCallback — данные кнопки «купить». Разбирается ДО карты запросов
// inline-генераций: там ключи случайные, и наш постоянный в неё не попадает.
const aiBuyCallback = "aibuy"

// aiBuyStartParam — хвост диплинка. Кнопка в группе ведёт в личку, потому что
// платёжная карточка на виду у всего чата — так себе зрелище, да и платит
// человек всё равно один.
const aiBuyStartParam = "stars"

// aiBuyPayload едет в счёт и возвращается в успешном платеже. По нему и только
// по нему решается, сколько начислить: доверять сумме из апдейта нельзя, а
// цена к моменту оплаты могла и смениться.
const aiBuyPayload = "ai-credits"

// starsPack is how many generations one purchase gives.
func starsPack() int {
	return envInt("AI_STARS_PACK", starsPackDefault)
}

// starsPrice is what one pack costs, in stars.
func starsPrice() int {
	return envInt("AI_STARS_PRICE", starsPriceDefault)
}

func envInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		log.Printf("[warn] %s = %q — не число, беру %d\n", name, raw, fallback)

		return fallback
	}

	return value
}

// aiBuyOfferText is the line added under the refusal.
//
// Цену называем, а квоту по-прежнему нет: цена одна для всех, а квота у каждого
// своя, и озвучивать её в группе значит затевать спор о том, почему у соседа
// больше (см. aiLimitFallbackText).
func aiBuyOfferText() string {
	return fmt.Sprintf("\n\nМожно не ждать: %d генераций за %d ⭐. Не сгорают.",
		starsPack(), starsPrice())
}

// aiBuyKeyboard is the button under the refusal.
//
// В личке это обычная кнопка — счёт приходит сюда же. В группе кнопка не
// нажимается ботом, а уводит в личку диплинком: платёжную карточку в общий чат
// слать незачем.
func aiBuyKeyboard(private bool) models.ReplyMarkup {
	label := fmt.Sprintf("%d генераций за %d ⭐", starsPack(), starsPrice())

	button := models.InlineKeyboardButton{Text: label, CallbackData: aiBuyCallback}
	if !private {
		button = models.InlineKeyboardButton{
			Text: label,
			URL:  fmt.Sprintf("https://t.me/%s?start=%s", botName, aiBuyStartParam),
		}
	}

	return models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{{button}},
	}
}

// aiBuyPointlessText answers somebody with no limit at all.
//
// Такой человек в отказ не упирается и купленное не потратит никогда. Взять с
// него деньги — значит продать пустоту, и возвращать её потом руками.
const aiBuyPointlessText = "Тебе покупать нечего: лимита на тебе нет, " +
	"генерации и так не кончаются."

// aiBuyWorthIt says whether selling this person anything makes sense.
//
// Ошибку базы считаем «можно»: отказать покупателю из-за икоты базы хуже, чем
// изредка продать лишнее — купленное не сгорает и дождётся своего часа.
func aiBuyWorthIt(userID int64) bool {
	limit, err := model.GetAiLimit(userID)
	if err != nil {
		log.Println("[error] лимит для покупки не прочитался:", err)

		return true
	}

	return limit.IsActive()
}

// sendAiInvoice puts the bill in the person's private chat.
//
// Всегда в личку, даже если нажали в группе: ChatID здесь — id человека, а не
// чата. У Telegram это одно и то же число для приватной переписки.
func sendAiInvoice(ctx context.Context, b *bot.Bot, userID int64) error {
	pack, price := starsPack(), starsPrice()

	_, err := b.SendInvoice(ctx, &bot.SendInvoiceParams{
		ChatID:      userID,
		Title:       fmt.Sprintf("%d генераций", pack),
		Description: "Сверх бесплатного лимита. Не сгорают — тратятся по мере того, как лимит кончается.",
		Payload:     aiBuyPayload,
		Currency:    starsCurrency,
		Prices: []models.LabeledPrice{
			{Label: fmt.Sprintf("%d генераций", pack), Amount: price},
		},
	})

	return err
}

// processAiBuyCallback answers the button under the refusal.
func processAiBuyCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	from := update.CallbackQuery.From
	msg := update.CallbackQuery.Message.Message
	if !aiBuyWorthIt(from.ID) {
		if msg != nil {
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          msg.Chat.ID,
				Text:            aiBuyPointlessText,
				ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
			})
			utils.ProcessSendMessageError(err, msg.Chat.ID)
		}

		return
	}

	if err := sendAiInvoice(ctx, b, from.ID); err != nil {
		// Самая частая причина — человек не начинал переписку с ботом, и слать
		// ему в личку нельзя. Говорим это там, где он нажал, иначе ответ
		// уедет туда, куда он всё равно не смотрит.
		log.Println("[error] счёт не ушёл:", err)
		if msg != nil {
			_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          msg.Chat.ID,
				Text:            fmt.Sprintf("Не смог прислать счёт в личку. Напиши мне туда @%s и нажми ещё раз.", botName),
				ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
			})
			utils.ProcessSendMessageError(sendErr, msg.Chat.ID)
		}
	}
}

// processAiBuyStart greets whoever arrived by the deep link from a group.
func processAiBuyStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatID := update.Message.Chat.ID
	reply := func(text string) {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
		utils.ProcessSendMessageError(err, chatID)
	}

	if !aiBuyWorthIt(update.Message.From.ID) {
		reply(aiBuyPointlessText)

		return
	}

	if err := sendAiInvoice(ctx, b, update.Message.From.ID); err != nil {
		log.Println("[error] счёт по диплинку не ушёл:", err)
		reply(serverDeadText)
	}
}

// processPreCheckout confirms the purchase.
//
// Telegram ждёт ответа десять секунд и без него отменяет платёж, поэтому
// здесь не должно быть ничего тяжёлого. Проверяем только, что счёт наш:
// payload свой, валюта звёздная.
func processPreCheckout(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.PreCheckoutQuery
	ok := query.InvoicePayload == aiBuyPayload && query.Currency == starsCurrency

	params := &bot.AnswerPreCheckoutQueryParams{PreCheckoutQueryID: query.ID, OK: ok}
	if !ok {
		log.Printf("[warn] чужой счёт: payload=%q currency=%q\n",
			query.InvoicePayload, query.Currency)
		params.ErrorMessage = "Этот счёт уже недействителен. Попробуй ещё раз."
	}

	if _, err := b.AnswerPreCheckoutQuery(ctx, params); err != nil {
		log.Println("[error] не ответил на pre_checkout:", err)
	}
}

// processSuccessfulPayment credits the pack and says so.
//
// Начисляем по НАШЕМУ размеру пачки, а не по сумме платежа: сумма приходит от
// Telegram в звёздах, и пересчитывать её обратно в генерации значило бы делить
// на цену, которая могла смениться между выставлением счёта и оплатой.
func processSuccessfulPayment(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	payment := message.SuccessfulPayment
	chatID := message.Chat.ID
	userID := message.From.ID
	pack := starsPack()

	reply := func(text string) {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
		utils.ProcessSendMessageError(err, chatID)
	}

	fresh, err := model.CreditAiPurchase(payment.TelegramPaymentChargeID, userID,
		payment.TotalAmount, pack)
	if err != nil {
		// Деньги Telegram уже взял, а начислить мы не смогли. Молчать тут
		// нельзя: человек должен знать, что идти с этим к нам, а id платежа —
		// то единственное, по чему потом делается возврат.
		log.Printf("[error] оплата %s от %d не начислена: %v\n",
			payment.TelegramPaymentChargeID, userID, err)
		reply("Оплата прошла, а начислить не вышло — уже разбираюсь. " +
			"Номер платежа: " + payment.TelegramPaymentChargeID)

		return
	}

	if !fresh {
		// Повтор того же апдейта. Второй раз не начисляем и вторым сообщением
		// не беспокоим.
		log.Printf("оплата %s уже была начислена\n", payment.TelegramPaymentChargeID)

		return
	}

	log.Printf("оплата: %d ⭐ от %s, начислено %d генераций\n",
		payment.TotalAmount, utils.GetAnyName(message.From), pack)

	balance := pack
	if credit, err := model.GetAiCredit(userID); err == nil && credit != nil {
		balance = credit.Balance
	}

	reply(fmt.Sprintf("Начислил %d генераций. Всего на счету: %d. "+
		"Тратятся, только когда кончается бесплатный лимит.", pack, balance))
}
