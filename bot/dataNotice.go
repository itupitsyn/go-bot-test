package bot

import (
	"context"
	"fmt"
	"telebot/model"
	"telebot/utils"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// dataNoticeRevision датирует формулировку.
//
// Нужна не для вида: текст говорит «продолжая пользоваться, ты
// соглашаешься», а соглашаться можно только с чем-то определённым. Дата и
// есть то определённое — какая именно редакция висела в боте. Меняешь
// текст — меняй дату, иначе строка про согласие теряет смысл.
const dataNoticeRevision = "13.10.2026"

// dataNoticeText is what the bot says about the data it handles.
//
// Why it lives in code and not in the phrazes table like the other wordings:
// this one must be stable and reviewable. A phrase key is picked at random out
// of a group and can be edited to anything by whoever has the admin panel —
// fine for "жди теперь", wrong for a statement about personal data, which has
// to say the same thing today and in a month and has to match what the code
// actually does.
//
// EVERY NUMBER HERE IS A REAL SETTING, not a round figure for show. If any of
// them changes, this text is wrong and has to change with it:
//
//   - 6 часов  — RESULT_TTL_HOURS в сервисе (result_store.TTL_SECONDS)
//   - 7 суток  — COMFY_OUTPUT_TTL_DAYS там же, и model.InlineImageTTL здесь
//   - 30 суток — STATS_RETENTION_DAYS в сервисе
//   - 5 картинок — model.InlineImageLimit
//
// Про «продолжая пользоваться, ты соглашаешься»: это акцепт ПРАВИЛ
// пользования поведением, и для правил он работает (ГК РФ ст. 428,
// договор присоединения). Согласием на обработку персональных данных по
// 152-ФЗ он НЕ является: ст. 9 требует конкретного и осознанного
// согласия, а для фотографий лица (биометрия, ст. 11) — письменного.
// Молчаливое продолжение пользования ни тем, ни другим не является.
// Строка полезна, но зафиксированное согласие не заменяет: для него нужна
// явная отметка пользователя и запись факта со временем и редакцией.
//
// Про «во внешние сервисы ничего не уходит»: проверено 13.10.2026 — в коде бота
// нет ни одного внешнего адреса, всё считается на своём железе. Появится
// сторонний сервис — эту строку надо снимать первой.
func dataNoticeText() string {
	return "<b>Фото и данные</b>\n" +
		"Обрабатываем присланное фото и текст запроса — ровно для того, чтобы " +
		"сделать то, что ты попросил, и ни для чего больше.\n\n" +
		"Считается всё на нашем собственном железе: во внешние сервисы ничего " +
		"не уходит.\n\n" +
		"<b>Саму картинку мы не храним.</b> В базе лежит только идентификатор " +
		"файла от Telegram — меньше сотни байт, — а сам файл остаётся на " +
		"стороне Telegram.\n\n" +
		"<b>Сколько живут следы</b>\n" +
		"— готовый результат — не дольше 6 часов;\n" +
		"— служебные копии на диске — до 7 суток;\n" +
		"— запись о запросе вместе с его текстом — до 30 суток;\n" +
		fmt.Sprintf("— буфер твоих картинок для inline — последние %d, "+
			"пропадают через %d суток без обращений.\n\n",
			model.InlineImageLimit, int(model.InlineImageTTL.Hours()/24)) +
		"<b>Как убрать раньше</b>\n" +
		"/forget — очистить буфер своих картинок сразу\n" +
		"/stickers_delete — удалить свой набор стикеров. Сам набор хранится у " +
		"Telegram, а не у нас.\n\n" +
		"<b>Чего бот не делает</b>\n" +
		"Не рисует обнажённое и сексуальное и не берёт в работу фотографии " +
		"детей — такие запросы отклоняются.\n\n" +
		"<b>Согласие</b>\n" +
		"Продолжая пользоваться ботом, ты соглашаешься с тем, что описано " +
		"выше. Не хочешь — не присылай фото: рисовать по описанию бот " +
		"умеет и без них.\n" +
		"Редакция от " + dataNoticeRevision + "."
}

// processDataNotice answers /privacy.
//
// The same text is pointed at from /ai_help rather than copied into it: the
// help is already long, and a statement about data should have one address to
// link to, not two versions drifting apart.
func processDataNotice(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := update.Message.Chat.ID

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatId,
		Text:            dataNoticeText(),
		ParseMode:       models.ParseModeHTML,
		ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
	})
	utils.ProcessSendMessageError(err, chatId)
}
