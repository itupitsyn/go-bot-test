package model

import (
	"telebot/database"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAiMaintenanceIsActive(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	earlier := now.Add(-time.Hour)

	cases := []struct {
		name        string
		maintenance *AiMaintenance
		want        bool
	}{
		{"no row", nil, false},
		{"disabled", &AiMaintenance{IsEnabled: false, EndsAt: &later}, false},
		{"enabled without end", &AiMaintenance{IsEnabled: true}, true},
		{"enabled until later", &AiMaintenance{IsEnabled: true, EndsAt: &later}, true},
		{"enabled but over", &AiMaintenance{IsEnabled: true, EndsAt: &earlier}, false},
	}

	for _, c := range cases {
		if got := c.maintenance.IsActive(now); got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}

// Until the admin panel has saved the setting there is no row, which means "no
// maintenance".
func TestGetAiMaintenanceMissingRow(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectQuery(`SELECT \* FROM "ai_maintenances"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "is_enabled", "ends_at", "updated_at"}))

	maintenance, err := GetAiMaintenance()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if maintenance != nil {
		t.Errorf("want nil maintenance, got %+v", maintenance)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
