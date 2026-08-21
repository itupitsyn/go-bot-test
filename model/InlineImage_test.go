package model

import (
	"encoding/base64"
	"errors"
	"regexp"
	"telebot/database"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

var errTrim = errors.New("trim failed")

// Saving a picture upserts it on (user_id, file_unique_id), so that re-sending
// the same file refreshes it instead of taking a second slot in the buffer, and
// trims the buffer down to the limit in the same transaction.
func TestSaveInlineImageUpsertsAndTrims(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "inline_images".*ON CONFLICT`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`DELETE FROM inline_images`).
		WithArgs(int64(123), int64(123), InlineImageLimit).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	if err := SaveInlineImage(123, "file", "thumb", "unique"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// A failing trim rolls the whole thing back, so the buffer never grows past the
// limit on a half-applied save.
func TestSaveInlineImageRollsBackFailedTrim(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "inline_images"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`DELETE FROM inline_images`).
		WillReturnError(errTrim)
	mock.ExpectRollback()

	if err := SaveInlineImage(123, "file", "thumb", "unique"); err == nil {
		t.Fatal("want an error from a failing trim, got none")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// A preview URL must not move once handed out — Telegram caches previews by
// URL — so re-sending a known picture refreshes everything except its token.
func TestSaveInlineImageKeepsTokenOnConflict(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectBegin()
	// Anchoring on RETURNING pins the whole SET list: the token showing up in it
	// would push the match past the end and fail the test.
	mock.ExpectQuery(`DO UPDATE SET "file_id"=.*,"thumb_file_id"=.*,"updated_at"="excluded"."updated_at" RETURNING`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`DELETE FROM inline_images`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := SaveInlineImage(123, "file", "thumb", "unique"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Two pictures never share a preview URL, and a token is wide enough that it
// cannot be guessed by walking the endpoint.
func TestNewInlineImageToken(t *testing.T) {
	first, err := newInlineImageToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	second, err := newInlineImageToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if first == second {
		t.Error("want two different tokens")
	}

	decoded, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil {
		t.Fatalf("want a URL-safe token, got %q: %v", first, err)
	}
	if len(decoded) != inlineImageTokenBytes {
		t.Errorf("want %d random bytes, got %d", inlineImageTokenBytes, len(decoded))
	}
}

// An empty token is what pictures saved before previews existed carry, and it
// must never resolve to one of them.
func TestGetInlineImageByEmptyToken(t *testing.T) {
	db, _ := database.ConnectToMockDB()
	Init(db)

	if _, err := GetInlineImageByToken(""); err == nil {
		t.Error("want an empty token to resolve to nothing")
	}
}

// Pictures come back most recently touched first: the buffer rotates on
// updated_at, so a picture reused today outlives one uploaded yesterday.
func TestGetInlineImagesOrdersByUpdatedAt(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	rows := sqlmock.NewRows([]string{"id", "user_id", "file_id", "file_unique_id", "created_at", "updated_at"}).
		AddRow(2, int64(123), "fresh", "unique-2", time.Now(), time.Now()).
		AddRow(1, int64(123), "stale", "unique-1", time.Now(), time.Now().Add(-time.Hour))
	mock.ExpectQuery(regexp.QuoteMeta(`ORDER BY updated_at DESC, id DESC`)).WillReturnRows(rows)

	images, err := GetInlineImages(123, InlineImageLimit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(images) != 2 {
		t.Fatalf("want 2 images, got %d", len(images))
	}
	if images[0].FileID != "fresh" {
		t.Errorf("want the freshest image first, got %q", images[0].FileID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// The TTL sweep goes by updated_at as well, so a picture stays as long as it is
// being used and only expires once it is left alone.
func TestDeleteOldInlineImagesGoesByUpdatedAt(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	Init(db)

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "inline_images" WHERE updated_at < \$1`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	deleted, err := DeleteOldInlineImages()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 2 {
		t.Errorf("want 2 deleted images, got %d", deleted)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
