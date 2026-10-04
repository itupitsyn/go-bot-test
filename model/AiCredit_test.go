package model

import (
	"regexp"
	"telebot/database"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Остаток считает база, а не мы: иначе два платежа, пришедшие разом, оба
// прочитали бы старое число и одно начисление пропало бы.
func TestAddAiCreditCountsInTheDatabase(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`ON CONFLICT \("user_id"\) DO UPDATE SET "balance"=ai_credits\.balance \+`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42))
	mock.ExpectCommit()

	if err := AddAiCredit(42, 20); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Нулевое и отрицательное начисление до базы не доходит вовсе: вызвать его с
// нулём — обычное дело (вернуть нечего), и ходить за этим в базу незачем.
func TestAddAiCreditIgnoresNothing(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	for _, units := range []int{0, -5} {
		if err := AddAiCredit(42, units); err != nil {
			t.Fatalf("%d: неожиданная ошибка: %v", units, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Проверка остатка и списание — ОДИН запрос. Если их разнести, две команды,
// отправленные одновременно, увидят один и тот же остаток и потратят его
// дважды.
func TestSpendAiCreditChecksAndSubtractsAtOnce(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "ai_credits" SET "balance"=balance - $1`)).
		WithArgs(10, sqlmock.AnyArg(), 42, 10).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	ok, err := SpendAiCredit(42, 10)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !ok {
		t.Error("списание не состоялось, хотя строка обновилась")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Не хватило — база не трогает ни одной строки, и это единственный признак,
// по которому мы узнаём отказ.
func TestSpendAiCreditSaysNoWhenShort(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "ai_credits"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	ok, err := SpendAiCredit(42, 10)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if ok {
		t.Error("списание засчитано, хотя строк не обновилось")
	}
}

// Повтор того же платежа не начисляет второй раз: Telegram переприсылает
// апдейт, который считает недоставленным.
func TestCreditAiPurchaseIsIdempotent(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "ai_purchases"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	fresh, err := CreditAiPurchase("charge-1", 42, 5, 20)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if fresh {
		t.Error("повторный платёж принят за новый")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Новый платёж записывается И начисляется, обе записи в одной транзакции:
// падение между ними оставило бы деньги взятыми, а генерации невыданными.
func TestCreditAiPurchaseRecordsAndGrants(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "ai_purchases"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO "ai_credits"`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42))
	mock.ExpectCommit()

	fresh, err := CreditAiPurchase("charge-2", 42, 5, 20)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !fresh {
		t.Error("новый платёж принят за повтор")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
