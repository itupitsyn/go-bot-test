package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"telebot/model"
	"telebot/utils"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Платные баллы поверх лимита.
//
// Лимит ставит админ отдельным людям, и до сих пор упереться в него значило
// просто подождать. Теперь у упёршегося есть второй выход: купить пачку
// баллов за Telegram Stars. Бесплатная квота при этом не меняется — см.
// model.AiCredit о том, почему это отдельный карман, а не прибавка к окну.
//
// Кто НЕ видит предложения: все, у кого лимита нет. Таких подавляющее
// большинство, они в отказ не упираются вовсе, и показывать им ценник не за
// что — они и так ничего не ждут.
//
// Цена команды в баллах — в aiPoints.go, и менять её надо там: здесь только
// то, за сколько баллы продаются.

// starsPackage — одна пачка: сколько баллов и за сколько звёзд.
type starsPackage struct {
	Points int
	Stars  int
}

// starsLadderDefault — лестница пачек на 24.10.2026.
//
// Крупные дешевле за балл, и это не щедрость: человеку, которому нужно
// шестьсот баллов в сутки, пачка по сотне означает шесть нажатий «купить»
// каждый день — он уйдёт раньше, чем кончатся баллы.
//
// Нижняя ступень — десятая звезды за балл, то есть около четверти доллара за
// час карты. Примерно за столько та же карта сдаётся в аренду на стороне;
// ниже начинается работа в подарок.
var starsLadderDefault = []starsPackage{
	{Points: 100, Stars: 10},
	{Points: 600, Stars: 50},
	{Points: 2000, Stars: 150},
	{Points: 6000, Stars: 400},
}

// maxPackPoints — выше этого числа пачка считается испорченной.
//
// Защита не от человека, а от опечатки в окружении: лишний нуль в
// AI_STARS_LADDER иначе раздаёт шестьдесят тысяч баллов за четыреста звёзд, и
// узнаем мы об этом из счёта за электричество.
const maxPackPoints = 100000

// starsCurrency — валюта Telegram Stars. Единственная, у которой провайдерский
// токен пустой: деньги принимает сам Telegram.
const starsCurrency = "XTR"

// aiBuyCallback — приставка данных кнопки «купить». За ней идёт размер пачки:
// «aibuy:600». Разбирается ДО карты запросов inline-генераций: там ключи
// uuid-ы, двоеточий в них нет, и наша приставка в них не попадает.
const aiBuyCallback = "aibuy"

// aiBuyStartParam — хвост диплинка. Кнопка в группе ведёт в личку, потому что
// платёжная карточка на виду у всего чата — так себе зрелище, да и платит
// человек всё равно один.
const aiBuyStartParam = "stars"

// aiBuyPayload — приставка payload счёта. За ней, через двоеточие, едет размер
// пачки, и по нему — и только по нему — решается, сколько начислить.
const aiBuyPayload = "ai-credits"

// legacyPackPoints — во что превращается оплата по счёту, выписанному ДО
// баллов.
//
// У такого счёта в payload нет числа, а продавалось по нему двадцать
// генераций — то есть двадцать роликов. По той же мерке, по которой
// пересчитывались остатки на счетах (ролик — десять баллов), это двести
// баллов. Такой счёт может прийти ещё долго: человек держит карточку
// неоплаченной сколько хочет.
const legacyPackPoints = 200

// starsLadder is the packs on offer.
//
// Через окружение (AI_STARS_LADDER="100:10,600:50"), потому что цену нащупывают
// живьём, а пересобирать бота ради одной цифры не хочется. Испорченная строка
// отбрасывается ЦЕЛИКОМ: половина лестницы хуже прежней.
func starsLadder() []starsPackage {
	raw := strings.TrimSpace(os.Getenv("AI_STARS_LADDER"))
	if raw == "" {
		return starsLadderDefault
	}

	ladder := make([]starsPackage, 0, len(starsLadderDefault))
	for _, item := range strings.Split(raw, ",") {
		points, stars, ok := strings.Cut(strings.TrimSpace(item), ":")
		if !ok {
			log.Printf("[warn] AI_STARS_LADDER: в %q нет двоеточия, беру лестницу по умолчанию\n", item)

			return starsLadderDefault
		}

		pack := starsPackage{}
		pack.Points, ok = positiveInt(points, maxPackPoints)
		if !ok {
			log.Printf("[warn] AI_STARS_LADDER: %q — не размер пачки, беру лестницу по умолчанию\n", points)

			return starsLadderDefault
		}

		// Потолок цены — тот же: звёзд за пачку не бывает больше, чем баллов
		// в самой крупной, и опечатка в цене так же дорога, как в размере.
		pack.Stars, ok = positiveInt(stars, maxPackPoints)
		if !ok {
			log.Printf("[warn] AI_STARS_LADDER: %q — не цена, беру лестницу по умолчанию\n", stars)

			return starsLadderDefault
		}

		ladder = append(ladder, pack)
	}

	if len(ladder) == 0 {
		return starsLadderDefault
	}

	return ladder
}

// positiveInt reads a number that must be positive and within bounds.
func positiveInt(raw string, max int) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 || value > max {
		return 0, false
	}

	return value, true
}

// aiBuyPayloadFor builds the payload of an invoice for this pack.
func aiBuyPayloadFor(points int) string {
	return fmt.Sprintf("%s:%d", aiBuyPayload, points)
}

// pointsFromPayload reads how much was sold out of the invoice's payload.
//
// Начисляем по payload, а не по сумме платежа: сумма приходит в звёздах, и
// делить её обратно на цену значило бы доверять цене, которая могла смениться
// между выставлением счёта и оплатой. Payload же сочиняем мы сами, а выписать
// счёт от имени бота больше некому.
func pointsFromPayload(payload string) (int, bool) {
	if payload == aiBuyPayload {
		return legacyPackPoints, true
	}

	rest, ok := strings.CutPrefix(payload, aiBuyPayload+":")
	if !ok {
		return 0, false
	}

	return positiveInt(rest, maxPackPoints)
}

// packFromCallback finds which button was pressed.
//
// Сверяемся с лестницей, а не берём число из кнопки: данные кнопки приходят
// от клиента, и подменить их самодельным клиентом ничего не стоит. Кнопка без
// числа — из сообщения, отправленного ДО лестницы; такие висят в чатах и
// нажимаются до сих пор, по ним отдаём самую мелкую пачку.
func packFromCallback(data string) (starsPackage, bool) {
	ladder := starsLadder()
	if data == aiBuyCallback {
		return ladder[0], true
	}

	rest, ok := strings.CutPrefix(data, aiBuyCallback+":")
	if !ok {
		return starsPackage{}, false
	}

	points, ok := positiveInt(rest, maxPackPoints)
	if !ok {
		return starsPackage{}, false
	}

	for _, pack := range ladder {
		if pack.Points == points {
			return pack, true
		}
	}

	return starsPackage{}, false
}

// aiBuyOfferText is the line added under the refusal.
//
// Называем только нижнюю ступень: ценник из четырёх строк под отказом читается
// как прейскурант, а остальное человек увидит на кнопках. Квоту по-прежнему не
// называем — цена одна для всех, а квота у каждого своя, и озвучивать её в
// группе значит затевать спор о том, почему у соседа больше (см.
// aiLimitFallbackText).
func aiBuyOfferText() string {
	first := starsLadder()[0]

	return fmt.Sprintf("\n\nМожно не ждать: %d баллов за %d ⭐, есть пачки крупнее. Не сгорают.",
		first.Points, first.Stars)
}

// aiBuyKeyboard is the buttons under the refusal.
//
// В личке это пачки списком — счёт приходит сюда же. В группе кнопка не
// нажимается ботом, а уводит в личку диплинком: платёжную карточку в общий чат
// слать незачем.
func aiBuyKeyboard(private bool) models.ReplyMarkup {
	if !private {
		return models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{{{
				Text: "Купить баллы",
				URL:  fmt.Sprintf("https://t.me/%s?start=%s", botName, aiBuyStartParam),
			}}},
		}
	}

	ladder := starsLadder()
	rows := make([][]models.InlineKeyboardButton, 0, len(ladder))
	for _, pack := range ladder {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         fmt.Sprintf("%d баллов — %d ⭐", pack.Points, pack.Stars),
			CallbackData: fmt.Sprintf("%s:%d", aiBuyCallback, pack.Points),
		}})
	}

	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
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

// sendAiInvoice puts the bill for this pack in the person's private chat.
//
// Всегда в личку, даже если нажали в группе: ChatID здесь — id человека, а не
// чата. У Telegram это одно и то же число для приватной переписки.
func sendAiInvoice(ctx context.Context, b *bot.Bot, userID int64, pack starsPackage) error {
	_, err := b.SendInvoice(ctx, &bot.SendInvoiceParams{
		ChatID:      userID,
		Title:       fmt.Sprintf("%d баллов", pack.Points),
		Description: "Сверх бесплатного лимита. Не сгорают — тратятся по мере того, как лимит кончается.",
		Payload:     aiBuyPayloadFor(pack.Points),
		Currency:    starsCurrency,
		Prices: []models.LabeledPrice{
			{Label: fmt.Sprintf("%d баллов", pack.Points), Amount: pack.Stars},
		},
	})

	return err
}

// processAiBuyCallback answers a pack button.
func processAiBuyCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	from := update.CallbackQuery.From
	msg := update.CallbackQuery.Message.Message
	reply := func(text string) {
		if msg == nil {
			return
		}

		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			Text:            text,
			ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
		})
		utils.ProcessSendMessageError(err, msg.Chat.ID)
	}

	if !aiBuyWorthIt(from.ID) {
		reply(aiBuyPointlessText)

		return
	}

	pack, ok := packFromCallback(update.CallbackQuery.Data)
	if !ok {
		// Кнопка от лестницы, которой больше нет: цену сменили, а сообщение
		// со старыми кнопками осталось в чате.
		log.Printf("[warn] пачка %q не из нынешней лестницы\n", update.CallbackQuery.Data)
		reply("Эта пачка больше не продаётся. Попроси генерацию ещё раз — покажу нынешние.")

		return
	}

	if err := sendAiInvoice(ctx, b, from.ID, pack); err != nil {
		// Самая частая причина — человек не начинал переписку с ботом, и слать
		// ему в личку нельзя. Говорим это там, где он нажал, иначе ответ
		// уедет туда, куда он всё равно не смотрит.
		log.Println("[error] счёт не ушёл:", err)
		reply(fmt.Sprintf("Не смог прислать счёт в личку. Напиши мне туда @%s и нажми ещё раз.", botName))
	}
}

// processAiBuyStart greets whoever arrived by the deep link from a group.
//
// Счёт сразу не выписываем: по диплинку не узнать, какую пачку хотели — кнопка
// в группе одна на все. Показываем лестницу здесь, в личке.
func processAiBuyStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatID := update.Message.Chat.ID

	if !aiBuyWorthIt(update.Message.From.ID) {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: aiBuyPointlessText})
		utils.ProcessSendMessageError(err, chatID)

		return
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        "Баллы не сгорают и тратятся, только когда кончается бесплатный лимит. Выбирай пачку:",
		ReplyMarkup: aiBuyKeyboard(true),
	})
	utils.ProcessSendMessageError(err, chatID)
}

// processPreCheckout confirms the purchase.
//
// Telegram ждёт ответа десять секунд и без него отменяет платёж, поэтому
// здесь не должно быть ничего тяжёлого. Проверяем только, что счёт наш:
// payload свой и читается, валюта звёздная.
func processPreCheckout(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.PreCheckoutQuery
	_, known := pointsFromPayload(query.InvoicePayload)
	ok := known && query.Currency == starsCurrency

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
func processSuccessfulPayment(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	payment := message.SuccessfulPayment
	chatID := message.Chat.ID
	userID := message.From.ID

	reply := func(text string) {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
		utils.ProcessSendMessageError(err, chatID)
	}

	points, ok := pointsFromPayload(payment.InvoicePayload)
	if !ok {
		// Деньги взяты по счёту, которого мы не узнаём. Начислять наугад
		// нельзя, молчать нельзя тем более: id платежа — то единственное, по
		// чему делается возврат.
		log.Printf("[error] оплата %s от %d: payload %q не читается\n",
			payment.TelegramPaymentChargeID, userID, payment.InvoicePayload)
		reply("Оплата прошла, а начислить не вышло — уже разбираюсь. " +
			"Номер платежа: " + payment.TelegramPaymentChargeID)

		return
	}

	fresh, err := model.CreditAiPurchase(payment.TelegramPaymentChargeID, userID,
		payment.TotalAmount, points)
	if err != nil {
		// Та же причина: сказать человеку, что идти с этим к нам.
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

	log.Printf("оплата: %d ⭐ от %s, начислено %d баллов\n",
		payment.TotalAmount, utils.GetAnyName(message.From), points)

	balance := points
	if credit, err := model.GetAiCredit(userID); err == nil && credit != nil {
		balance = credit.Balance
	}

	reply(fmt.Sprintf("Начислил %d баллов. Всего на счету: %d. "+
		"Тратятся, только когда кончается бесплатный лимит.", points, balance))
}
