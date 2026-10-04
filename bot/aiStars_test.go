package bot

import (
	"strings"
	"telebot/database"
	"telebot/model"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-telegram/bot/models"
)

// Цена и размер пачки берутся из окружения, потому что их нащупывают живьём.
// Мусор в переменной не должен ронять продажу — откатываемся на дефолт.
func TestEnvIntFallsBackOnJunk(t *testing.T) {
	cases := map[string]int{
		"":      7,
		"20":    20,
		"ноль":  7,
		"0":     7,
		"-3":    7,
		"3.5":   7,
		" 20 ":  7,
		"99999": 99999,
	}

	for raw, want := range cases {
		t.Setenv("AI_STARS_TEST", raw)
		if got := envInt("AI_STARS_TEST", 7); got != want {
			t.Errorf("%q: получили %d, ждали %d", raw, got, want)
		}
	}
}

// В личке кнопка зовёт счёт сюда же, в группе уводит в личку: платёжная
// карточка на виду у всего чата — так себе зрелище, и платит всё равно один.
func TestAiBuyKeyboardLeadsOutOfAGroup(t *testing.T) {
	botName = "testbot"

	private := aiBuyKeyboard(true).(models.InlineKeyboardMarkup).InlineKeyboard[0][0]
	if private.CallbackData != aiBuyCallback {
		t.Errorf("в личке ждали кнопку-колбэк, получили %+v", private)
	}
	if private.URL != "" {
		t.Errorf("в личке ссылка не нужна: %q", private.URL)
	}

	group := aiBuyKeyboard(false).(models.InlineKeyboardMarkup).InlineKeyboard[0][0]
	if group.CallbackData != "" {
		t.Errorf("в группе колбэк не нужен: %q", group.CallbackData)
	}
	if want := "https://t.me/testbot?start=" + aiBuyStartParam; group.URL != want {
		t.Errorf("ссылка %q, ждали %q", group.URL, want)
	}
}

// Цену называем, а квоту по-прежнему нет: цена одна для всех, а квота у
// каждого своя, и её озвучивание в группе — повод для спора.
func TestAiBuyOfferNamesThePriceOnly(t *testing.T) {
	t.Setenv("AI_STARS_PACK", "20")
	t.Setenv("AI_STARS_PRICE", "5")

	got := aiBuyOfferText()
	for _, want := range []string{"20", "5", "⭐"} {
		if !strings.Contains(got, want) {
			t.Errorf("в предложении нет %q: %q", want, got)
		}
	}
	if !strings.Contains(got, "сгорают") {
		t.Errorf("не сказано, что не сгорают: %q", got)
	}
}

// Кому лимит не ставили, тому и покупать нечего: купленное он не потратит
// никогда, и деньги пришлось бы возвращать руками.
func TestAiBuyWorthItNeedsALimit(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	mock.ExpectQuery(`SELECT \* FROM "ai_limits"`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount", "period"}).
			AddRow(42, 20, model.AiPeriodHour))
	if !aiBuyWorthIt(42) {
		t.Error("человеку с лимитом продавать можно")
	}

	mock.ExpectQuery(`SELECT \* FROM "ai_limits"`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount", "period"}))
	if aiBuyWorthIt(43) {
		t.Error("человеку без лимита продавать нечего")
	}

	// Икота базы не должна мешать покупке: купленное не сгорает и дождётся.
	mock.ExpectQuery(`SELECT \* FROM "ai_limits"`).WillReturnError(errDatabaseDown)
	if !aiBuyWorthIt(44) {
		t.Error("на ошибке базы продажу запрещать не надо")
	}
}

// setUnits зовут и там, где списания не было вовсе. Падать на этом нельзя:
// иначе отказ по лимиту уронил бы обработчик.
func TestAiChargeSetUnitsSurvivesNothing(t *testing.T) {
	var none *aiCharge
	none.setUnits(0)

	(&aiCharge{}).setUnits(3)
}
