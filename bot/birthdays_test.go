package bot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"telebot/model"
	"telebot/utils"
	"testing"
	"time"

	"github.com/go-telegram/bot"
)

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()
	date, err := time.ParseInLocation(time.DateOnly, value, birthdayZone)
	if err != nil {
		t.Fatal(err)
	}

	return date
}

func TestParseBirthdayDay(t *testing.T) {
	for text, want := range map[string]string{"15.03": "03-15", "1.2": "02-01", "29.02": "02-29"} {
		if got, ok := parseBirthdayDay(text); !ok || got != want {
			t.Errorf("parseBirthdayDay(%q) = %q, %v, want %q", text, got, ok, want)
		}
	}

	for _, text := range []string{"", "завтра", "15", "32.01", "31.04", "10.13", "0.1", "1.0", "15.03.1990"} {
		if got, ok := parseBirthdayDay(text); ok {
			t.Errorf("parseBirthdayDay(%q) = %q, want a refusal", text, got)
		}
	}
}

func TestFormatBirthdayPhrase(t *testing.T) {
	if got := formatBirthdayPhrase("{name}, с днём рождения, {name}!", "@vasya"); got != "@vasya, с днём рождения, @vasya!" {
		t.Errorf("got %q", got)
	}
	if got := formatBirthdayPhrase("С днём рождения,", "@vasya"); got != "С днём рождения, @vasya" {
		t.Errorf("a phrase without the placeholder must still name the person, got %q", got)
	}
}

func TestBirthdayPhrazes(t *testing.T) {
	got := birthdayPhrazes(nil, birthdayTodayDefault, "@vasya")
	if len(got) != 1 || got[0].Value != "@vasya, с днём рождения!" || !got[0].IsHTML {
		t.Errorf("an empty phrase section must fall back to the default, got %+v", got)
	}

	link := birthdayMention(10, "", "Вася")
	stored := []model.Phraze{{Value: "1 < 2, {name} & всё"}, {Value: "{name}?", IsWithSpoiler: true}}
	got = birthdayPhrazes(stored, birthdayTodayDefault, link)
	if want := "1 &lt; 2, " + link + " &amp; всё"; got[0].Value != want || !got[0].IsHTML {
		t.Errorf("the phrase must be escaped and the link must survive, got %q", got[0].Value)
	}
	if got[1].Value != link+"?" || !got[1].IsWithSpoiler {
		t.Errorf("got %+v", got[1])
	}
	if stored[0].Value != "1 < 2, {name} & всё" || stored[0].IsHTML {
		t.Error("the stored phrases must stay untouched")
	}
}

func TestBirthdayMention(t *testing.T) {
	cases := []struct{ name, alternative, want string }{
		{"vasya", "Вася", "@vasya"},
		{"", "Вася", `<a href="tg://user?id=10">Вася</a>`},
		{"", "<b>Вася&", `<a href="tg://user?id=10">&lt;b&gt;Вася&amp;</a>`},
		{"", "", `<a href="tg://user?id=10">Именинник</a>`},
	}
	for _, item := range cases {
		if got := birthdayMention(10, item.name, item.alternative); got != item.want {
			t.Errorf("birthdayMention(%q, %q) = %q, want %q", item.name, item.alternative, got, item.want)
		}
	}
}

// Что реально уходит в Telegram: ссылка работает, только если parse_mode задан.
func TestBirthdayMentionReachesTelegramAsHTML(t *testing.T) {
	var text, parseMode string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		text, parseMode = r.FormValue("text"), r.FormValue("parse_mode")
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer server.Close()

	b, err := bot.New("1:token", bot.WithServerURL(server.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}

	phrazes := birthdayPhrazes(nil, birthdayTodayDefault, birthdayMention(10, "", "Вася"))
	if err := utils.SendPhrazes(context.Background(), b, &model.Chat{ID: -100}, phrazes); err != nil {
		t.Fatal(err)
	}

	if parseMode != "HTML" || text != `<a href="tg://user?id=10">Вася</a>, с днём рождения!` {
		t.Errorf("parse_mode = %q, text = %q", parseMode, text)
	}
}

func TestBirthdayPersonName(t *testing.T) {
	if got := birthdayPersonName("vasya", "Вася"); got != "@vasya" {
		t.Errorf("got %q", got)
	}
	if got := birthdayPersonName("", "Вася"); got != "Вася" {
		t.Errorf("got %q", got)
	}
}

func TestBirthdayDaysOnLeapYears(t *testing.T) {
	if got := birthdayDaysOn(mustDate(t, "2027-02-28")); len(got) != 2 || got[1] != "02-29" {
		t.Errorf("common year: got %v", got)
	}
	if got := birthdayDaysOn(mustDate(t, "2028-02-28")); len(got) != 1 {
		t.Errorf("leap year 28th: got %v", got)
	}
	if got := birthdayDaysOn(mustDate(t, "2028-02-29")); len(got) != 1 || got[0] != "02-29" {
		t.Errorf("leap year 29th: got %v", got)
	}
}

func TestBirthdayNotices(t *testing.T) {
	cases := []struct {
		name     string
		row      model.BirthdayRow
		today    string
		count    int
		tomorrow bool
	}{
		{"the day before", model.BirthdayRow{Birthday: "03-15"}, "2026-03-14", 1, true},
		{"on the day", model.BirthdayRow{Birthday: "03-15"}, "2026-03-15", 1, false},
		{"the day after", model.BirthdayRow{Birthday: "03-15"}, "2026-03-16", 0, false},
		{"over the new year", model.BirthdayRow{Birthday: "01-01"}, "2026-12-31", 1, true},
		{"29 Feb, reminder in a common year", model.BirthdayRow{Birthday: "02-29"}, "2027-02-27", 1, true},
		{"29 Feb, greeting in a common year", model.BirthdayRow{Birthday: "02-29"}, "2027-02-28", 1, false},
		{"already reminded", model.BirthdayRow{Birthday: "03-15", RemindedOn: "2026-03-14"}, "2026-03-14", 0, false},
		{"already congratulated", model.BirthdayRow{Birthday: "03-15", CongratulatedOn: "2026-03-15"}, "2026-03-15", 0, false},
		{"last year's greeting does not count", model.BirthdayRow{Birthday: "03-15", CongratulatedOn: "2026-03-15"}, "2027-03-15", 1, false},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got := birthdayNotices([]model.BirthdayRow{item.row}, mustDate(t, item.today))
			if len(got) != item.count {
				t.Fatalf("got %d notices, want %d", len(got), item.count)
			}
			if item.count == 1 && got[0].tomorrow != item.tomorrow {
				t.Errorf("tomorrow = %v, want %v", got[0].tomorrow, item.tomorrow)
			}
		})
	}
}

func TestIsBirthdayCommand(t *testing.T) {
	botName = "Kalovrat_Bot"

	for _, text := range []string{"/birthday @vasya 15.03", "/birthday", "/birthday@kalovrat_bot @vasya 15.03"} {
		if !isBirthdayCommand(text, "/birthday") {
			t.Errorf("%q should match /birthday", text)
		}
	}
	for _, text := range []string{"/birthdays", "/birthday_remind @vasya", "/birthday@other_bot x", "", "привет"} {
		if isBirthdayCommand(text, "/birthday") {
			t.Errorf("%q should not match /birthday", text)
		}
	}
}

func TestBirthdayNoRetry(t *testing.T) {
	joined := func(err error) error { return errors.Join(fmt.Errorf("%w, kicked", err)) }

	if !birthdayNoRetry(joined(bot.ErrorForbidden)) || !birthdayNoRetry(joined(bot.ErrorBadRequest)) {
		t.Error("a chat the bot cannot write to must not be retried")
	}
	if birthdayNoRetry(joined(bot.ErrorTooManyRequests)) || birthdayNoRetry(errors.New("network")) {
		t.Error("a passing failure must stay due")
	}
}
