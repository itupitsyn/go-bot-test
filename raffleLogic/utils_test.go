package raffleLogic

import (
	"database/sql/driver"
	"telebot/database"
	"telebot/model"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPhraze(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	rows := sqlmock.NewRows([]string{"rey", "value"}).AddRows([]driver.Value{AcceptPrizeKey, "phraze1"}, []driver.Value{AcceptPrizeKey, "phraze2"})
	mock.ExpectQuery("^SELECT \\* FROM \"phrazes\" WHERE key = \\$1").WillReturnRows(rows)

	if randomPhraze := GetRandomPhrazeByKey(AcceptPrizeKey); randomPhraze != "phraze1" && randomPhraze != "phraze2" {
		t.Errorf("want %s or %s, got %s", "phraze1", "phraze2", randomPhraze)
	}
}

func TestGetPrizeName(t *testing.T) {
	prize := model.Prize{
		Name: "Prize name",
	}

	prizeName := GetPrizeName(&prize)

	if prizeName != "Prize name" {
		t.Errorf("want %s, got %s", "Prize name", prizeName)
	}
}

func TestGetDefaultPrizeName(t *testing.T) {
	prize := model.Prize{
		Name: "",
	}

	prizeName := GetPrizeName(&prize)

	if prizeName != defaultPrizeName {
		t.Errorf("want %s, got %s", defaultPrizeName, prizeName)
	}
}
