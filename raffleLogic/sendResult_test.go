package raffleLogic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"telebot/database"
	"telebot/model"
	"telebot/utils"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-telegram/bot"
	"gorm.io/datatypes"
)

// Строка с победителем после перехода на SendPhrazes должна уйти как раньше: HTML,
// иначе ссылка на человека без ника покажется в чате разметкой.
func TestSendResultGoesOutAsHTML(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)
	mock.ExpectQuery(`SELECT .* FROM "prizes"`).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("<3 пицца"))

	var text, parseMode string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		text, parseMode = r.FormValue("text"), r.FormValue("parse_mode")
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer server.Close()

	b, err := bot.New("1:token", bot.WithServerURL(server.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}

	winner := utils.MentionHTML(10, "", "<Вася>", "Победитель")
	date := datatypes.Date(time.Now().UTC())
	if err := SendResult(context.Background(), b, &model.Chat{ID: -100}, date, winner); err != nil {
		t.Fatal(err)
	}

	want := `<a href="tg://user?id=10">&lt;Вася&gt;</a> выигрывает &lt;3 пицца!!`
	if parseMode != "HTML" || text != want {
		t.Errorf("parse_mode = %q, text = %q, want %q", parseMode, text, want)
	}
}

// Сбой отправки должен доходить до вызывающего, как и до перехода.
func TestSendResultReportsFailure(t *testing.T) {
	db, mock := database.ConnectToMockDB()
	model.Init(db)
	mock.ExpectQuery(`SELECT .* FROM "prizes"`).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("пицца"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"ok":false,"error_code":403,"description":"Forbidden: bot was kicked"}`)
	}))
	defer server.Close()

	b, err := bot.New("1:token", bot.WithServerURL(server.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}

	date := datatypes.Date(time.Now().UTC())
	if err := SendResult(context.Background(), b, &model.Chat{ID: -100}, date, "@vasya"); err == nil {
		t.Error("want an error")
	}
}
