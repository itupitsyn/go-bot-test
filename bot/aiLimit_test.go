package bot

import (
	"errors"
	"telebot/database"
	"telebot/model"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

var errDatabaseDown = errors.New("database is down")

// Своя формулировка перебивает общую, и за общей мы в базу даже не ходим:
// лишний запрос на каждый отказ ни к чему.
func TestAiLimitRefusalTextPrefersOwn(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	got := aiLimitRefusalText(&model.AiLimit{Message: "Тебе — нельзя."})
	if got != "Тебе — нельзя." {
		t.Errorf("want the personal wording, got %q", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestAiLimitRefusalTextFallsBackToGeneral(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	mock.ExpectQuery(`SELECT \* FROM "ai_limit_settings"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "message"}).AddRow(1, "Хорош уже."))

	got := aiLimitRefusalText(&model.AiLimit{Message: ""})
	if got != "Хорош уже." {
		t.Errorf("want the general wording, got %q", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Админка ещё ничего не сохраняла — строки нет, и это не ошибка.
func TestAiLimitRefusalTextFallsBackToOurs(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	mock.ExpectQuery(`SELECT \* FROM "ai_limit_settings"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "message"}))

	got := aiLimitRefusalText(nil)
	if got != aiLimitFallbackText {
		t.Errorf("want the built-in wording, got %q", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Пустая строка в настройках — это «своего текста нет», а не «отвечать пустым
// сообщением»: Telegram такое и не отправит.
func TestAiLimitRefusalTextIgnoresEmptyGeneral(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	mock.ExpectQuery(`SELECT \* FROM "ai_limit_settings"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "message"}).AddRow(1, ""))

	if got := aiLimitRefusalText(nil); got != aiLimitFallbackText {
		t.Errorf("want the built-in wording, got %q", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Лимита нет — ничего не списываем и в таблицу расхода не пишем. Иначе на
// каждую картинку в общем чате появлялась бы строка, которой никто не просил.
func TestChargeAiLimitWithoutLimit(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	mock.ExpectQuery(`SELECT \* FROM "ai_limits"`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount", "period", "message"}))

	usage, text, ok := chargeAiLimit(42, aiKindImage, 1)
	if !ok {
		t.Error("want the command allowed")
	}
	if usage != nil || text != "" {
		t.Errorf("want no charge and no text, got %+v %q", usage, text)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Набор стикеров стоит десять, и если до потолка осталось меньше — отказ, а
// расход не трогаем.
func TestChargeAiLimitRefusesWhenPackDoesNotFit(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	mock.ExpectQuery(`SELECT \* FROM "ai_limits"`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount", "period", "message"}).
			AddRow(42, 20, model.AiPeriodHour, "Остынь."))
	mock.ExpectQuery(`SELECT SUM\(units\) FROM "ai_usages"`).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(15))

	usage, text, ok := chargeAiLimit(42, aiKindSticker, 10)
	if ok {
		t.Error("want the command refused: 15 + 10 > 20")
	}
	if usage != nil {
		t.Errorf("want nothing charged, got %+v", usage)
	}
	if text != "Остынь." {
		t.Errorf("want the personal wording, got %q", text)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Ровно впритык — это ещё можно: отказ начинается с превышения, а не с
// достижения потолка.
func TestChargeAiLimitAllowsExactFit(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	mock.ExpectQuery(`SELECT \* FROM "ai_limits"`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount", "period", "message"}).
			AddRow(42, 20, model.AiPeriodHour, ""))
	mock.ExpectQuery(`SELECT SUM\(units\) FROM "ai_usages"`).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(10))
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "ai_usages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	_, _, ok := chargeAiLimit(42, aiKindSticker, 10)
	if !ok {
		t.Error("want the command allowed: 10 + 10 == 20")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// База молчит — пропускаем. Лимит это хозяйственная мелочь, и молчащий из-за
// неё бот хуже одной лишней картинки.
func TestChargeAiLimitLetsThroughOnDatabaseError(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	mock.ExpectQuery(`SELECT \* FROM "ai_limits"`).
		WillReturnError(errDatabaseDown)

	_, _, ok := chargeAiLimit(42, aiKindImage, 1)
	if !ok {
		t.Error("want the command allowed when the database is unreachable")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
