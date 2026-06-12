package model

import (
	"telebot/database"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// When a size already exists for the user today, the stored value is returned
// and no new value is generated/inserted.
func TestGetOrCreateTodaySizeExisting(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	rows := sqlmock.NewRows([]string{"user_id", "date", "value"}).
		AddRow(int64(123), time.Now(), 42)
	mock.ExpectQuery(`SELECT \* FROM "sizes"`).WillReturnRows(rows)

	size, err := GetOrCreateTodaySize(123, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if size != 42 {
		t.Errorf("want stored size 42, got %d", size)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
