package bot

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"telebot/model"
	"telebot/raffleLogic"
	"telebot/utils"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	birthdayTomorrowKey = "birthday_tomorrow"
	birthdayTodayKey    = "birthday_today"
	birthdayPlaceholder = "{name}"

	// Сообщения ждут этого часа по Москве, чтобы не будить чат в полночь. Если
	// бот в это время лежал, они уйдут, как только он встанет, но в те же сутки.
	birthdayHour = 10
	birthdayTick = time.Minute

	birthdayTomorrowDefault = "Завтра день рождения: {name}"
	birthdayTodayDefault    = "{name}, с днём рождения!"

	birthdayUsage = "Формат: /birthday @ник 15.03"
)

// Зона задана числом, а не именем: базы часовых поясов может не быть на машине,
// а смещение у Москвы постоянное.
var birthdayZone = time.FixedZone("MSK", 3*60*60)

var daysInMonth = []int{31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}

type birthdayNotice struct {
	row      model.BirthdayRow
	tomorrow bool
}

// parseBirthdayDay превращает «15.03» в «03-15». Год не принимаем и не храним.
func parseBirthdayDay(text string) (string, bool) {
	parts := strings.Split(text, ".")
	if len(parts) != 2 {
		return "", false
	}
	day, dayErr := strconv.Atoi(parts[0])
	month, monthErr := strconv.Atoi(parts[1])
	if dayErr != nil || monthErr != nil || month < 1 || month > 12 || day < 1 || day > daysInMonth[month-1] {
		return "", false
	}

	return fmt.Sprintf("%02d-%02d", month, day), true
}

// formatBirthdayDay — обратное: «03-15» в «15.03».
func formatBirthdayDay(day string) string {
	parts := strings.Split(day, "-")
	if len(parts) != 2 {
		return day
	}

	return parts[1] + "." + parts[0]
}

// birthdayDaysOn отвечает, чей день рождения приходится на эти сутки. В невисокосный
// год родившихся 29 февраля поздравляем 28-го.
func birthdayDaysOn(date time.Time) []string {
	day := date.Format("01-02")
	nextDay := date.AddDate(0, 0, 1).Format("01-02")
	if day == "02-28" && nextDay == "03-01" {
		return []string{day, "02-29"}
	}

	return []string{day}
}

func containsDay(days []string, day string) bool {
	for _, item := range days {
		if item == day {
			return true
		}
	}

	return false
}

// birthdayNotices выбирает, что ещё не отправлено сегодня.
func birthdayNotices(rows []model.BirthdayRow, today time.Time) []birthdayNotice {
	date := today.Format(time.DateOnly)
	todayDays := birthdayDaysOn(today)
	tomorrowDays := birthdayDaysOn(today.AddDate(0, 0, 1))

	notices := []birthdayNotice{}
	for _, row := range rows {
		if containsDay(todayDays, row.Birthday) {
			if row.CongratulatedOn != date {
				notices = append(notices, birthdayNotice{row: row})
			}
		} else if containsDay(tomorrowDays, row.Birthday) && row.RemindedOn != date {
			notices = append(notices, birthdayNotice{row: row, tomorrow: true})
		}
	}

	return notices
}

// birthdayPersonName: ник зовёт человека в чат, а без ника остаётся только имя.
func birthdayPersonName(name string, alternativeName string) string {
	if name != "" {
		return "@" + name
	}

	return alternativeName
}

// formatBirthdayPhrase подставляет имя, как formatSizePhrase подставляет размер: нет
// метки в тексте фразы — имя дописывается в конец.
func formatBirthdayPhrase(template string, name string) string {
	if strings.Contains(template, birthdayPlaceholder) {
		return strings.ReplaceAll(template, birthdayPlaceholder, name)
	}

	return fmt.Sprintf("%s %s", template, name)
}

// birthdayPhrazes — сценарий из раздела фраз, с именем внутри. Фраза со спойлером
// уходит как HTML, поэтому там имя экранируется: оно написано самим человеком.
func birthdayPhrazes(stored []model.Phraze, fallback string, name string) []model.Phraze {
	if len(stored) == 0 {
		stored = []model.Phraze{{Value: fallback}}
	}

	phrazes := make([]model.Phraze, 0, len(stored))
	for _, phraze := range stored {
		personName := name
		if phraze.IsWithSpoiler {
			personName = html.EscapeString(name)
		}
		phraze.Value = formatBirthdayPhrase(phraze.Value, personName)
		phrazes = append(phrazes, phraze)
	}

	return phrazes
}

// birthdayNoRetry: бота выгнали или чата больше нет, повтор не поможет. Остальное —
// сеть или лимит Telegram — останется в очереди до следующего круга.
func birthdayNoRetry(err error) bool {
	return errors.Is(err, bot.ErrorForbidden) || errors.Is(err, bot.ErrorBadRequest)
}

func sendBirthdayNotice(ctx context.Context, b *bot.Bot, notice birthdayNotice, date string) {
	row := notice.row
	chat, err := model.GetChatById(row.ChatID)
	if err != nil {
		log.Println("[error] error getting chat for a birthday", err)
		return
	}

	key, fallback := birthdayTodayKey, birthdayTodayDefault
	if notice.tomorrow {
		key, fallback = birthdayTomorrowKey, birthdayTomorrowDefault
	}
	stored := raffleLogic.GetRandomPhrazeByKey(key, chat.IsUncensored)
	name := birthdayPersonName(row.Name, row.AlternativeName)

	if err := utils.SendPhrazes(ctx, b, chat, birthdayPhrazes(stored, fallback, name)); err != nil && !birthdayNoRetry(err) {
		return
	}

	if notice.tomorrow {
		err = model.MarkBirthdayReminded(row.ChatID, row.UserID, date)
	} else {
		err = model.MarkBirthdayCongratulated(row.ChatID, row.UserID, date)
	}
	if err != nil {
		log.Println("[error] error marking a birthday as sent", err)
	}
}

func deliverBirthdays(ctx context.Context, b *bot.Bot, now time.Time) {
	today := now.In(birthdayZone)
	if today.Hour() < birthdayHour {
		return
	}

	days := append(birthdayDaysOn(today), birthdayDaysOn(today.AddDate(0, 0, 1))...)
	rows, err := model.GetBirthdaysOn(days)
	if err != nil {
		log.Println("[error] error getting birthdays", err)
		return
	}

	for _, notice := range birthdayNotices(rows, today) {
		sendBirthdayNotice(ctx, b, notice, today.Format(time.DateOnly))
	}
}

// WatchBirthdays раз в минуту смотрит, не пора ли кого-то поздравить. Первый проход
// сразу при запуске: бот мог проспать утро.
func WatchBirthdays(ctx context.Context, b *bot.Bot) {
	for {
		deliverBirthdays(ctx, b, time.Now())

		select {
		case <-ctx.Done():
			return
		case <-time.After(birthdayTick):
		}
	}
}

// isBirthdayCommand сверяет первое слово целиком: /birthday не должна срабатывать на /birthdays.
func isBirthdayCommand(msgTextLower string, command string) bool {
	fields := strings.Fields(msgTextLower)
	if len(fields) == 0 {
		return false
	}

	return fields[0] == command || fields[0] == command+"@"+strings.ToLower(botName)
}

func replyBirthday(ctx context.Context, b *bot.Bot, update *models.Update, text string) {
	chatId := update.Message.Chat.ID
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatId,
		Text:            text,
		ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
	})
	utils.ProcessSendMessageError(err, chatId)
}

// findBirthdayPerson ищет человека по нику из команды. Не нашёл — сам говорит об этом
// в чат и возвращает nil.
func findBirthdayPerson(ctx context.Context, b *bot.Bot, update *models.Update, nick string) *model.User {
	name := strings.TrimLeft(nick, "@")
	if name == "" {
		replyBirthday(ctx, b, update, birthdayUsage)
		return nil
	}

	user, err := model.GetUserByName(name)
	if err == nil {
		return user
	}
	if model.IsNotFound(err) {
		replyBirthday(ctx, b, update, fmt.Sprintf(
			"Про @%s у меня ничего нет. Пусть напишет что-нибудь в чат, и тогда можно будет задать дату.", name))
		return nil
	}

	log.Println("[error] error looking a user up for a birthday", err)
	replyBirthday(ctx, b, update, "Не вышло, попробуй позже")
	return nil
}

// processSetBirthday: /birthday @ник 15.03
func processSetBirthday(ctx context.Context, b *bot.Bot, update *models.Update) {
	if checkSetCommandInitiator(ctx, b, update) != nil {
		return
	}

	parts := strings.Fields(update.Message.Text)
	if len(parts) != 3 {
		replyBirthday(ctx, b, update, birthdayUsage)
		return
	}
	day, ok := parseBirthdayDay(parts[2])
	if !ok {
		replyBirthday(ctx, b, update, birthdayUsage)
		return
	}

	user := findBirthdayPerson(ctx, b, update, parts[1])
	if user == nil {
		return
	}
	if err := model.SetUserBirthday(user.ID, day); err != nil {
		log.Println("[error] error saving a birthday", err)
		replyBirthday(ctx, b, update, "Не вышло, попробуй позже")
		return
	}

	replyBirthday(ctx, b, update, fmt.Sprintf("Записал: @%s, %s", user.Name, parts[2]))
}

// processAddBirthdayReminder: /birthday_remind @ник. Без даты напоминать не о чем, и
// суперадмин должен узнать об этом сейчас, а не в день, когда никто не поздравит.
func processAddBirthdayReminder(ctx context.Context, b *bot.Bot, update *models.Update) {
	if checkSetCommandInitiator(ctx, b, update) != nil {
		return
	}

	parts := strings.Fields(update.Message.Text)
	if len(parts) != 2 {
		replyBirthday(ctx, b, update, "Формат: /birthday_remind @ник")
		return
	}

	user := findBirthdayPerson(ctx, b, update, parts[1])
	if user == nil {
		return
	}
	if user.Birthday == nil || *user.Birthday == "" {
		replyBirthday(ctx, b, update, fmt.Sprintf(
			"Не знаю, когда день рождения у @%s. Сначала задай дату: /birthday @%s 15.03", user.Name, user.Name))
		return
	}

	if err := model.AddBirthdayReminder(update.Message.Chat.ID, user.ID); err != nil {
		log.Println("[error] error adding a birthday reminder", err)
		replyBirthday(ctx, b, update, "Не вышло, попробуй позже")
		return
	}

	replyBirthday(ctx, b, update, fmt.Sprintf(
		"Буду напоминать про @%s, %s", user.Name, formatBirthdayDay(*user.Birthday)))
}

// processRemoveBirthdayReminder: /birthday_forget @ник
func processRemoveBirthdayReminder(ctx context.Context, b *bot.Bot, update *models.Update) {
	if checkSetCommandInitiator(ctx, b, update) != nil {
		return
	}

	parts := strings.Fields(update.Message.Text)
	if len(parts) != 2 {
		replyBirthday(ctx, b, update, "Формат: /birthday_forget @ник")
		return
	}

	user := findBirthdayPerson(ctx, b, update, parts[1])
	if user == nil {
		return
	}

	removed, err := model.RemoveBirthdayReminder(update.Message.Chat.ID, user.ID)
	if err != nil {
		log.Println("[error] error removing a birthday reminder", err)
		replyBirthday(ctx, b, update, "Не вышло, попробуй позже")
		return
	}
	if !removed {
		replyBirthday(ctx, b, update, fmt.Sprintf("Про @%s в этом чате я и не напоминал", user.Name))
		return
	}

	replyBirthday(ctx, b, update, fmt.Sprintf("Больше не напоминаю про @%s", user.Name))
}

// processBirthdays: /birthdays — список чата и состояние напоминаний.
func processBirthdays(ctx context.Context, b *bot.Bot, update *models.Update, chat *model.Chat) {
	if checkSetCommandInitiator(ctx, b, update) != nil {
		return
	}

	rows, err := model.GetChatBirthdays(chat.ID)
	if err != nil {
		log.Println("[error] error getting chat birthdays", err)
		replyBirthday(ctx, b, update, "Не вышло, попробуй позже")
		return
	}

	state := "выключены"
	if chat.BirthdaysEnabled {
		state = "включены"
	}
	lines := []string{"Напоминания: " + state, ""}
	if len(rows) == 0 {
		lines = append(lines, "Список пуст.")
	}
	for _, row := range rows {
		day := "дата не задана"
		if row.Birthday != "" {
			day = formatBirthdayDay(row.Birthday)
		}
		lines = append(lines, fmt.Sprintf("%s: %s", day, birthdayPersonName(row.Name, row.AlternativeName)))
	}
	lines = append(lines, "",
		"Дата: /birthday @ник 15.03",
		"Напоминать: /birthday_remind @ник",
		"Не напоминать: /birthday_forget @ник",
		"Включить или выключить: /birthdays_switch")

	replyBirthday(ctx, b, update, strings.Join(lines, "\n"))
}

// processBirthdaysSwitch: /birthdays_switch, переключатель как у /censorship.
func processBirthdaysSwitch(ctx context.Context, b *bot.Bot, update *models.Update, chat *model.Chat) {
	if checkSetCommandInitiator(ctx, b, update) != nil {
		return
	}

	text := "Напоминания о днях рождения включены"
	if chat.BirthdaysEnabled {
		text = "Напоминания о днях рождения выключены, список сохранён"
	}
	if err := chat.SetBirthdaysEnabled(!chat.BirthdaysEnabled); err != nil {
		log.Println("[error] error switching birthdays", err)
		text = "Не вышло, попробуй позже"
	}

	replyBirthday(ctx, b, update, text)
}
