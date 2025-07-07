package model

import (
	"telebot/database"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestUser(t *testing.T) {
	db, mock := database.ConnectToMockDB()

	rows := sqlmock.NewRows([]string{"key", "value"}).AddRow("kek", "penis")
	mock.ExpectQuery("^SELECT \\* FROM \"phrazes\" WHERE key = \\$1").WillReturnRows(rows)

	var result []Phraze

	if err := db.Model(&Phraze{}).Where("key = ?", "kek").Find(&result).Error; err != nil {
		t.Errorf("Penis error")
	}
}
