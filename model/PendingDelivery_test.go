package model

import (
	"database/sql/driver"
	"telebot/database"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// Повторная запись того же имени не должна плодить строки: задача на сервисе
// одна, и отдать её надо один раз.
func TestRememberDeliveryOverwritesTheSameJob(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "pending_deliveries".*ON CONFLICT \("job_id"\) DO UPDATE`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := RememberDelivery(PendingDelivery{
		JobID: "job-1", Kind: "video", ChatID: 7, UserID: 42, PaidUnits: 1,
	})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// recentTime принимает только время, похожее на «сейчас». Нужен, чтобы
// проверить отметку, которую ставит сам RememberDelivery: передать её в тест
// нельзя, а нулевое время сломало бы весь расчёт срока.
type recentTime struct{}

func (recentTime) Match(v driver.Value) bool {
	stamp, ok := v.(time.Time)
	if !ok || stamp.IsZero() {
		return false
	}

	return time.Since(stamp) < time.Minute && time.Since(stamp) > -time.Minute
}

// Время появления ставим сами, если его не передали: по нему потом решается,
// есть ли ещё смысл идти за результатом, а нулевое означало бы «брошено в 1
// году», то есть мгновенный отказ от любой задачи.
func TestRememberDeliveryStampsTheTime(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "pending_deliveries"`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), recentTime{}).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := RememberDelivery(PendingDelivery{JobID: "job-2", Kind: "image", ChatID: 7}); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestForgetDeliveryRemovesByJobID(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "pending_deliveries" WHERE job_id = \$1`).
		WithArgs("job-3").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := ForgetDelivery("job-3"); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Порядок важен: подбираем от старых к новым, потому что у старых меньше
// времени до того, как сервис выкинет результат.
func TestOwedDeliveriesComeOldestFirst(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	old := time.Now().Add(-2 * time.Hour)
	fresh := time.Now().Add(-time.Minute)

	mock.ExpectQuery(`SELECT \* FROM "pending_deliveries" ORDER BY created_at`).
		WillReturnRows(sqlmock.NewRows(
			[]string{"job_id", "kind", "chat_id", "reply_to_id", "wait_id", "user_id", "paid_units", "created_at"}).
			AddRow("job-old", "video", 7, 1, 2, 42, 1, old).
			AddRow("job-new", "image", 7, 3, 4, 42, 0, fresh))

	owed, err := OwedDeliveries()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(owed) != 2 {
		t.Fatalf("строк %d, ожидалось 2", len(owed))
	}
	if owed[0].JobID != "job-old" {
		t.Errorf("первой пришла %q, ожидалась самая старая", owed[0].JobID)
	}
	if owed[0].PaidUnits != 1 || owed[1].PaidUnits != 0 {
		t.Errorf("платные единицы не доехали: %d и %d", owed[0].PaidUnits, owed[1].PaidUnits)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
