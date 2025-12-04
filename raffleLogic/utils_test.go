package raffleLogic

import (
	"database/sql/driver"
	"telebot/database"
	"telebot/model"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPhrazeGroups(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	rows := sqlmock.NewRows([]string{"key", "value", "is_uncensored", "is_with_spoiler", "group", "order"})
	rows.AddRows([]driver.Value{AcceptPrizeKey, "phraze1", true, true, 1, 1}, []driver.Value{AcceptPrizeKey, "phraze2", true, true, 1, 2})
	rows.AddRows([]driver.Value{AcceptPrizeKey, "phraze3", true, true, 2, 1}, []driver.Value{AcceptPrizeKey, "phraze4", true, true, 2, 2})
	mock.ExpectQuery("^SELECT \\* FROM \"phrazes\" WHERE key = \\$1").WillReturnRows(rows)

	phrazes := GetRandomPhrazeByKey(AcceptPrizeKey, true)

	if len(phrazes) != 2 {
		t.Errorf("want len %d, got %d", 2, len(phrazes))
	}

	if phrazes[0].Value != "phraze1" && phrazes[0].Value != "phraze3" {
		t.Errorf("want len %s or %s, got %s", "phraze1", "phraze3", phrazes[0].Value)
	}
}

func TestPhrazes(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	rows := sqlmock.NewRows([]string{"key", "value", "is_uncensored", "is_with_spoiler", "group", "order"})
	rows.AddRows([]driver.Value{AcceptPrizeKey, "phraze1", true, true, nil, nil}, []driver.Value{AcceptPrizeKey, "phraze2", true, true, nil, nil})
	rows.AddRows([]driver.Value{AcceptPrizeKey, "phraze3", true, true, nil, nil}, []driver.Value{AcceptPrizeKey, "phraze4", true, true, nil, nil})
	mock.ExpectQuery("^SELECT \\* FROM \"phrazes\" WHERE key = \\$1").WillReturnRows(rows)

	phrazes := GetRandomPhrazeByKey(AcceptPrizeKey, true)

	if len(phrazes) != 1 {
		t.Errorf("want len %d, got %d", 1, len(phrazes))
	}

	if phrazes[0].Value != "phraze1" && phrazes[0].Value != "phraze2" && phrazes[0].Value != "phraze3" && phrazes[0].Value != "phraze4" {
		t.Errorf("want len %s or %s, got %s", "phraze1", "phraze3", phrazes[0].Value)
	}
}

func TestGetPrizeName(t *testing.T) {
	prize := model.Prize{
		Name: "Prize name",
	}

	prizeName := GetPrizeName(&prize, &model.Chat{})

	if prizeName != "Prize name" {
		t.Errorf("want %s, got %s", "Prize name", prizeName)
	}
}

func TestGetDefaultPrizeName(t *testing.T) {
	prize := model.Prize{
		Name: "",
	}

	prizeName := GetPrizeName(&prize, &model.Chat{IsUncensored: true})

	if prizeName != defaultPrizeNameUncensored {
		t.Errorf("want %s, got %s", defaultPrizeNameUncensored, prizeName)
	}

	prizeName = GetPrizeName(&prize, &model.Chat{IsUncensored: false})

	if prizeName != defaultPrizeName {
		t.Errorf("want %s, got %s", defaultPrizeName, prizeName)
	}
}
