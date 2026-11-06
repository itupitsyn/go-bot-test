package model

import (
	"errors"
	"telebot/database"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestReserveSearchQuota(t *testing.T) {
	now := time.Date(2026, 11, 30, 23, 30, 0, 0, time.FixedZone("MSK", 3*3600))
	cases := []struct {
		name     string
		affected int64
		want     bool
	}{
		{"under the limit", 1, true},
		{"limit reached", 0, false},
	}
	for _, c := range cases {
		db, mock := database.ConnectToMockDB()
		Init(db)
		// 23:30 MSK on the 30th is 20:30 UTC: still November.
		mock.ExpectExec(`INSERT INTO search_usages .* ON CONFLICT \(engine, month\) DO UPDATE .* WHERE search_usages.count < \$4`).
			WithArgs("brave", "2026-11", sqlmock.AnyArg(), 1900).
			WillReturnResult(sqlmock.NewResult(0, c.affected))

		got, err := ReserveSearchQuota("brave", 1900, now)
		if err != nil || got != c.want {
			t.Errorf("%s: got %v, %v, want %v", c.name, got, err, c.want)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func TestReserveSearchQuotaZeroLimitNeverAsks(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	got, err := ReserveSearchQuota("brave", 0, time.Now())
	if got || err != nil {
		t.Fatalf("a zero limit must refuse without a query: %v %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestReserveSearchQuotaError(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)
	mock.ExpectExec(`INSERT INTO search_usages`).WillReturnError(errors.New("db down"))

	got, err := ReserveSearchQuota("brave", 1900, time.Now())
	if got || err == nil {
		t.Fatalf("a database error must refuse and say so: %v %v", got, err)
	}
}
