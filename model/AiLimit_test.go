package model

import (
	"telebot/database"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAiLimitWindow(t *testing.T) {
	cases := []struct {
		period string
		want   time.Duration
	}{
		{AiPeriodHour, time.Hour},
		{AiPeriodDay, 24 * time.Hour},
		{AiPeriodWeek, 7 * 24 * time.Hour},
		// A typo in the database must not lock anybody out: an unknown period
		// gives no window, and no window reads as "no limit".
		{"fortnight", 0},
		{"", 0},
	}

	for _, c := range cases {
		if got := AiLimitWindow(c.period); got != c.want {
			t.Errorf("%q: want %v, got %v", c.period, c.want, got)
		}
	}
}

func TestAiLimitIsActive(t *testing.T) {
	cases := []struct {
		name  string
		limit *AiLimit
		want  bool
	}{
		{"no row", nil, false},
		{"zero amount", &AiLimit{Amount: 0, Period: AiPeriodHour}, false},
		{"negative amount", &AiLimit{Amount: -5, Period: AiPeriodHour}, false},
		{"unknown period", &AiLimit{Amount: 20, Period: "fortnight"}, false},
		{"set", &AiLimit{Amount: 20, Period: AiPeriodHour}, true},
	}

	for _, c := range cases {
		if got := c.limit.IsActive(); got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}

// Almost nobody has a limit, so a missing row is the normal answer, not a
// failure.
func TestGetAiLimitMissingRow(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectQuery(`SELECT \* FROM "ai_limits"`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "amount", "period", "message"}))

	limit, err := GetAiLimit(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if limit != nil {
		t.Errorf("want nil limit, got %+v", limit)
	}
	if limit.IsActive() {
		t.Error("a missing limit must not restrict anything")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// SUM over no rows is NULL, not 0, and reading it into a plain int would fail.
// Somebody who has spent nothing is the common case, so this path matters.
func TestCountAiUsageNothingSpent(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectQuery(`SELECT SUM\(units\) FROM "ai_usages"`).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(nil))

	used, err := CountAiUsage(42, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if used != 0 {
		t.Errorf("want 0, got %d", used)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestCountAiUsageSums(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectQuery(`SELECT SUM\(units\) FROM "ai_usages"`).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(13))

	used, err := CountAiUsage(42, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if used != 13 {
		t.Errorf("want 13, got %d", used)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// A pack that came out empty gives everything back, and the charge disappears
// rather than sitting there as a zero.
func TestSetUnitsZeroDeletes(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "ai_usages"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := (&AiUsage{ID: 7, UserID: 42}).SetUnits(0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Nothing was charged (the person has no limit), so there is nothing to give
// back and nothing to talk to the database about.
func TestSetUnitsOnNilCharge(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	var usage *AiUsage
	if err := usage.SetUnits(10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
