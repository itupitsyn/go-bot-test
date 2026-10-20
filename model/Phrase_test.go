package model

import (
	"regexp"
	"telebot/database"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// На уже переведённой базе миграция не должна трогать таблицу: иначе каждый
// запуск бота снимал бы и ставил первичный ключ заново.
func TestGivePhrazesAnIDLeavesAReadyTableAlone(t *testing.T) {
	db, mock := database.ConnectToMockDB()

	mock.ExpectQuery("FROM pg_constraint").
		WillReturnRows(sqlmock.NewRows([]string{"name", "columns"}).AddRow("phrazes_pkey", "id"))

	if err := GivePhrazesAnID(db); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Новая база: таблицы ещё нет, ключ запрос не нашёл. Создаст её AutoMigrate, и
// уже с нужным ключом.
func TestGivePhrazesAnIDSkipsAMissingTable(t *testing.T) {
	db, mock := database.ConnectToMockDB()

	mock.ExpectQuery("FROM pg_constraint").
		WillReturnRows(sqlmock.NewRows([]string{"name", "columns"}))

	if err := GivePhrazesAnID(db); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Старый ключ (key, value) меняется на id одной транзакцией, а имя снимаемого
// ограничения берётся то, которое вернула база.
func TestGivePhrazesAnIDSwapsTheOldKey(t *testing.T) {
	db, mock := database.ConnectToMockDB()

	mock.ExpectQuery("FROM pg_constraint").
		WillReturnRows(sqlmock.NewRows([]string{"name", "columns"}).AddRow("phrazes_old_pkey", "key,value"))

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`ALTER TABLE phrazes ADD COLUMN IF NOT EXISTS id bigserial`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`ALTER TABLE phrazes DROP CONSTRAINT "phrazes_old_pkey"`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`ALTER TABLE phrazes ADD PRIMARY KEY (id)`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := GivePhrazesAnID(db); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
