package model

import (
	"regexp"
	"telebot/database"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Фотография и правило про тело обязаны попасть в запись: без них переделать
// один стикер нечем — бот не знает, из чего рисовать и по какому правилу.
func TestSaveStickerPackKeepsThePhotoAndTheRule(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "sticker_packs"`)).
		WithArgs("emo42_by_bot", 10, "photo-file-id", true,
			sqlmock.AnyArg(), sqlmock.AnyArg(), int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42))
	mock.ExpectCommit()

	if err := SaveStickerPack(42, "emo42_by_bot", 10, "photo-file-id", true); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Повторный набор переписывает ту же строку, а не заводит вторую: набор у
// человека один, и ссылка на него должна оставаться прежней.
func TestSaveStickerPackUpdatesTheSameRow(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`ON CONFLICT \("user_id"\) DO UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42))
	mock.ExpectCommit()

	if err := SaveStickerPack(42, "emo42_by_bot", 9, "photo", false); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
