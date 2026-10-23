package bot

import (
	"strings"
	"telebot/database"
	"telebot/model"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-telegram/bot/models"
)

// Лестница берётся из окружения, потому что цену нащупывают живьём. Мусор в
// переменной не должен ронять продажу и не должен проезжать половиной: пачка
// без цены или с лишним нулём хуже прежней лестницы.
func TestStarsLadderFallsBackOnJunk(t *testing.T) {
	good := []starsPackage{{Points: 100, Stars: 10}, {Points: 600, Stars: 50}}
	cases := map[string][]starsPackage{
		"":                   starsLadderDefault,
		"100:10,600:50":      good,
		" 100 : 10 , 600:50": good,
		"100:10,600":         starsLadderDefault,
		"100:10,abc:50":      starsLadderDefault,
		"100:10,600:0":       starsLadderDefault,
		"0:10":               starsLadderDefault,
		"-100:10":            starsLadderDefault,
		"1000000:10":         starsLadderDefault,
	}

	for raw, want := range cases {
		t.Setenv("AI_STARS_LADDER", raw)
		got := starsLadder()
		if len(got) != len(want) {
			t.Errorf("%q: пачек %d, ждали %d", raw, len(got), len(want))

			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%q: пачка %d это %+v, ждали %+v", raw, i, got[i], want[i])
			}
		}
	}
}

// В личке кнопки зовут счёт сюда же, в группе одна кнопка уводит в личку:
// платёжная карточка на виду у всего чата — так себе зрелище, и платит всё
// равно один.
func TestAiBuyKeyboardLeadsOutOfAGroup(t *testing.T) {
	botName = "testbot"
	t.Setenv("AI_STARS_LADDER", "100:10,600:50")

	rows := aiBuyKeyboard(true).(models.InlineKeyboardMarkup).InlineKeyboard
	if len(rows) != 2 {
		t.Fatalf("в личке ждали пачку на кнопку, получили %d кнопок", len(rows))
	}
	if want := aiBuyCallback + ":100"; rows[0][0].CallbackData != want {
		t.Errorf("колбэк %q, ждали %q", rows[0][0].CallbackData, want)
	}
	if rows[0][0].URL != "" {
		t.Errorf("в личке ссылка не нужна: %q", rows[0][0].URL)
	}

	group := aiBuyKeyboard(false).(models.InlineKeyboardMarkup).InlineKeyboard
	if len(group) != 1 {
		t.Fatalf("в группе ждали одну кнопку, получили %d", len(group))
	}
	if group[0][0].CallbackData != "" {
		t.Errorf("в группе колбэк не нужен: %q", group[0][0].CallbackData)
	}
	if want := "https://t.me/testbot?start=" + aiBuyStartParam; group[0][0].URL != want {
		t.Errorf("ссылка %q, ждали %q", group[0][0].URL, want)
	}
}

// Под отказом называем только нижнюю ступень: прейскурант из четырёх строк
// там не нужен, остальное человек увидит на кнопках. Квоту не называем
// по-прежнему — она у каждого своя.
func TestAiBuyOfferNamesTheSmallestPack(t *testing.T) {
	t.Setenv("AI_STARS_LADDER", "100:10,600:50")

	got := aiBuyOfferText()
	for _, want := range []string{"100", "10", "⭐", "сгорают"} {
		if !strings.Contains(got, want) {
			t.Errorf("в предложении нет %q: %q", want, got)
		}
	}
	if strings.Contains(got, "600") {
		t.Errorf("крупные пачки под отказом не перечисляем: %q", got)
	}
}

// Начисляем по payload, а не по сумме платежа: сумма приходит в звёздах, и
// делить её обратно на цену значило бы доверять цене, которая могла смениться
// между выставлением счёта и оплатой.
func TestPointsFromPayload(t *testing.T) {
	cases := map[string]int{
		aiBuyPayloadFor(600): 600,
		aiBuyPayloadFor(100): 100,
		// Счёт, выписанный ДО баллов: числа в нём нет, а продавалось по нему
		// двадцать генераций — двадцать роликов.
		aiBuyPayload:          legacyPackPoints,
		aiBuyPayload + ":0":   0,
		aiBuyPayload + ":-5":  0,
		aiBuyPayload + ":abc": 0,
		aiBuyPayload + ":":    0,
		"чужой счёт":          0,
		"":                    0,
	}

	for payload, want := range cases {
		got, ok := pointsFromPayload(payload)
		if want == 0 {
			if ok {
				t.Errorf("%q: приняли как свой и дали %d", payload, got)
			}

			continue
		}
		if !ok || got != want {
			t.Errorf("%q: получили %d (ok=%v), ждали %d", payload, got, ok, want)
		}
	}
}

// Данные кнопки приходят от клиента, и подменить их самодельным клиентом
// ничего не стоит. Поэтому число сверяется с лестницей, а не берётся на веру:
// иначе «aibuy:6000» выписал бы счёт на шесть тысяч баллов по цене сотни.
func TestPackFromCallbackRejectsForgedNumbers(t *testing.T) {
	t.Setenv("AI_STARS_LADDER", "100:10,600:50")

	if pack, ok := packFromCallback(aiBuyCallback + ":600"); !ok || pack.Stars != 50 {
		t.Errorf("своя пачка не узналась: %+v (ok=%v)", pack, ok)
	}

	// Кнопка без числа — из сообщения, отправленного до лестницы. Такие висят
	// в чатах и нажимаются до сих пор.
	if pack, ok := packFromCallback(aiBuyCallback); !ok || pack.Points != 100 {
		t.Errorf("старая кнопка должна давать нижнюю ступень: %+v (ok=%v)", pack, ok)
	}

	for _, data := range []string{
		aiBuyCallback + ":123",
		aiBuyCallback + ":6000",
		aiBuyCallback + ":0",
		aiBuyCallback + ":",
		"garbage",
	} {
		if pack, ok := packFromCallback(data); ok {
			t.Errorf("%q: приняли как пачку %+v", data, pack)
		}
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
