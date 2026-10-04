package bot

import (
	"errors"
	"strings"
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

	charge, text, ok := chargeAiLimit(42, aiKindImage, 1)
	if !ok {
		t.Error("want the command allowed")
	}
	if charge != nil || text != "" {
		t.Errorf("want no charge and no text, got %+v %q", charge, text)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Набор стикеров стоит десять, и если до потолка осталось меньше — отказ, а
// расход не трогаем.
// Бесплатного не хватает, купленного нет — отказ с предложением. И квоту при
// этом не трогаем: команда не состоится целиком, отъедать за неё нечего.
func TestChargeAiLimitRefusesWhenPackDoesNotFit(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	mock.ExpectQuery(`SELECT \* FROM "ai_limits"`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount", "period", "message"}).
			AddRow(42, 20, model.AiPeriodHour, "Остынь."))
	mock.ExpectQuery(`SELECT SUM\(units\) FROM "ai_usages"`).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(15))
	// Купленного тоже нет: строк не обновилось.
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "ai_credits"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	charge, text, ok := chargeAiLimit(42, aiKindSticker, 10)
	if ok {
		t.Error("want the command refused: 15 + 10 > 20")
	}
	if charge != nil {
		t.Errorf("want nothing charged, got %+v", charge)
	}
	if !strings.HasPrefix(text, "Остынь.") {
		t.Errorf("want the personal wording first, got %q", text)
	}
	if !strings.Contains(text, "⭐") {
		t.Errorf("want the offer appended, got %q", text)
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

// Бесплатное кончилось, а купленное есть — работаем молча, без предложения.
// Это и есть вся суть платных генераций: человек не упирается в стену, пока у
// него на счету что-то лежит.
func TestChargeAiLimitSpendsBoughtUnits(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	mock.ExpectQuery(`SELECT \* FROM "ai_limits"`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount", "period", "message"}).
			AddRow(42, 20, model.AiPeriodHour, "Остынь."))
	mock.ExpectQuery(`SELECT SUM\(units\) FROM "ai_usages"`).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(20))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "ai_credits"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	charge, text, ok := chargeAiLimit(42, aiKindImage, 1)
	if !ok {
		t.Fatal("want the command allowed: it was paid for")
	}
	if text != "" {
		t.Errorf("want no refusal text, got %q", text)
	}
	if charge == nil || charge.paid != 1 || charge.userID != 42 {
		t.Errorf("want one paid unit charged to 42, got %+v", charge)
	}
	// Платное НЕ пишется в ai_usages: оно иначе раздуло бы окно и съело
	// бесплатную квоту следующего часа.
	if charge != nil && charge.usage != nil {
		t.Error("paid work must not be written into the free quota")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Бесплатного хватило — за купленным даже не ходим.
func TestChargeAiLimitDoesNotTouchCreditsWhileFreeLasts(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	mock.ExpectQuery(`SELECT \* FROM "ai_limits"`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount", "period", "message"}).
			AddRow(42, 20, model.AiPeriodHour, ""))
	mock.ExpectQuery(`SELECT SUM\(units\) FROM "ai_usages"`).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(5))
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "ai_usages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	charge, _, ok := chargeAiLimit(42, aiKindImage, 1)
	if !ok {
		t.Fatal("want the command allowed")
	}
	if charge == nil || charge.paid != 0 {
		t.Errorf("want nothing paid, got %+v", charge)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Платим только за то, что СВЕРХ лимита: пять бесплатных доедаются, за
// оставшиеся пять списывается со счёта. Без этого человек с девятью
// свободными генерациями платил бы за весь набор из десяти.
func TestChargeAiLimitSpendsFreeFirst(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	mock.ExpectQuery(`SELECT \* FROM "ai_limits"`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount", "period", "message"}).
			AddRow(42, 20, model.AiPeriodHour, ""))
	mock.ExpectQuery(`SELECT SUM\(units\) FROM "ai_usages"`).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(15))
	// Со счёта уходит ровно недостача — пять, а не десять.
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "ai_credits"`).
		WithArgs(5, sqlmock.AnyArg(), 42, 5).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// И столько же бесплатных записывается в окно.
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "ai_usages"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()

	charge, text, ok := chargeAiLimit(42, aiKindSticker, 10)
	if !ok {
		t.Fatalf("want the pack allowed, got refusal %q", text)
	}
	if charge == nil || charge.free != 5 || charge.paid != 5 {
		t.Errorf("want five free and five paid, got %+v", charge)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Неизрасходованное возвращается СНАЧАЛА в платный карман: квота всё равно
// растворится вместе с окном, а купленное лежит, пока не потратят.
func TestAiChargeRefundsMoneyFirst(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	// Было 5 бесплатных и 5 платных, израсходовано 7 — вернуть надо 3, и все
	// три платные.
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "ai_usages"`).
		WithArgs(5, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "ai_credits"`).
		WithArgs(3, sqlmock.AnyArg(), sqlmock.AnyArg(), 42, 3, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42))
	mock.ExpectCommit()

	charge := &aiCharge{usage: &model.AiUsage{ID: 1, UserID: 42},
		userID: 42, free: 5, paid: 5}
	charge.setUnits(7)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// А когда платного не хватает на весь возврат, остаток идёт в квоту.
func TestAiChargeRefundsTheRestIntoTheQuota(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	// Было 5 и 5, израсходовано 2 — вернуть 8: пять платных и три бесплатных,
	// от квоты остаётся два.
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "ai_usages"`).
		WithArgs(2, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "ai_credits"`).
		WithArgs(5, sqlmock.AnyArg(), sqlmock.AnyArg(), 42, 5, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42))
	mock.ExpectCommit()

	charge := &aiCharge{usage: &model.AiUsage{ID: 1, UserID: 42},
		userID: 42, free: 5, paid: 5}
	charge.setUnits(2)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Ничего не вышло вовсе — возвращается всё, и строка квоты удаляется.
func TestAiChargeGivesEverythingBack(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "ai_usages"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "ai_credits"`).
		WithArgs(4, sqlmock.AnyArg(), sqlmock.AnyArg(), 42, 4, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42))
	mock.ExpectCommit()

	charge := &aiCharge{usage: &model.AiUsage{ID: 1, UserID: 42},
		userID: 42, free: 6, paid: 4}
	charge.setUnits(0)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Пять картинок подряд, по одной, когда свободных осталось три: первые три
// проходят, четвёртая и пятая упираются в отказ с предложением купить.
//
// Это не то же самое, что набор на десять: там недостача видна сразу и
// команда не состоится целиком. Здесь каждая картинка — отдельная команда,
// и стена встаёт ровно там, где кончилась квота.
func TestChargeAiLimitOneByOneUntilTheQuotaRunsOut(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	// Лимит 20, потрачено 17 — свободно три.
	for _, used := range []int{17, 18, 19} {
		mock.ExpectQuery(`SELECT \* FROM "ai_limits"`).
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount", "period", "message"}).
				AddRow(42, 20, model.AiPeriodHour, "Остынь."))
		mock.ExpectQuery(`SELECT SUM\(units\) FROM "ai_usages"`).
			WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(used))
		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO "ai_usages"`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		mock.ExpectCommit()
	}

	// Четвёртая и пятая: свободного нет, на счету пусто.
	for i := 0; i < 2; i++ {
		mock.ExpectQuery(`SELECT \* FROM "ai_limits"`).
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount", "period", "message"}).
				AddRow(42, 20, model.AiPeriodHour, "Остынь."))
		mock.ExpectQuery(`SELECT SUM\(units\) FROM "ai_usages"`).
			WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(20))
		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "ai_credits"`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
	}

	allowed, refused := 0, 0
	for i := 1; i <= 5; i++ {
		charge, text, ok := chargeAiLimit(42, aiKindImage, 1)
		if ok {
			allowed++
			if charge == nil || charge.free != 1 || charge.paid != 0 {
				t.Errorf("запрос %d: ждали одну бесплатную, получили %+v", i, charge)
			}
			continue
		}

		refused++
		if !strings.Contains(text, "⭐") {
			t.Errorf("запрос %d: в отказе нет предложения: %q", i, text)
		}
	}

	if allowed != 3 || refused != 2 {
		t.Errorf("прошло %d, отказано %d — ждали 3 и 2", allowed, refused)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// То же самое, но на счету есть купленное: стена не встаёт, четвёртая и пятая
// уходят со счёта по одной.
func TestChargeAiLimitFallsThroughToCreditsOneByOne(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	for i := 0; i < 2; i++ {
		mock.ExpectQuery(`SELECT \* FROM "ai_limits"`).
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount", "period", "message"}).
				AddRow(42, 20, model.AiPeriodHour, ""))
		mock.ExpectQuery(`SELECT SUM\(units\) FROM "ai_usages"`).
			WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(20))
		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE "ai_credits"`).
			WithArgs(1, sqlmock.AnyArg(), 42, 1).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
	}

	for i := 1; i <= 2; i++ {
		charge, text, ok := chargeAiLimit(42, aiKindImage, 1)
		if !ok {
			t.Fatalf("запрос %d: отказ %q, а на счету есть", i, text)
		}
		if charge == nil || charge.paid != 1 || charge.free != 0 {
			t.Errorf("запрос %d: ждали одну платную, получили %+v", i, charge)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
