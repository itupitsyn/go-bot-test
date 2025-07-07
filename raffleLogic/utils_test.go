package raffleLogic

import (
	"database/sql/driver"
	"fmt"
	"telebot/database"
	"telebot/model"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPhraze(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)

	kek := GetRandomPhrazeByKey(AcceptPrizeKey)

	fmt.Println(kek)

	rows := sqlmock.NewRows([]string{"key", "value"}).AddRows([]driver.Value{"kek", "pek"}, []driver.Value{"kek", "shmek"})
	mock.ExpectQuery("^SELECT \\* FROM \"phrazes\" WHERE key = \\$1").WillReturnRows(rows)

	var result []model.Phraze

	if err := db.Model(&model.Phraze{}).Where("key = ?", "penis").Find(&result).Error; err != nil {
		t.Errorf("Penis error")
	}
}
