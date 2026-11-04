package bot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"telebot/database"
	"telebot/model"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func TestImageCaptionToExplain(t *testing.T) {
	cases := map[string]string{
		"":                   "",
		"Что тут?":           "",
		"  что тут  ":        "",
		"сократи":            "",
		"TLDR":               "",
		"Смотри какой кот":   "Смотри какой кот",
		"что тут происходит": "что тут происходит",
	}
	for caption, want := range cases {
		if got := imageCaptionToExplain(&models.Message{Caption: caption}); got != want {
			t.Errorf("imageCaptionToExplain(%q) = %q, want %q", caption, got, want)
		}
	}
}

func TestJoinExplanations(t *testing.T) {
	cases := []struct{ text, image, language, want string }{
		{"про кота", "кот на столе", "ru", "Текст: про кота\n\nКартинка: кот на столе"},
		{"about a cat", "a cat on a table", "en", "Text: about a cat\n\nImage: a cat on a table"},
		{"про кота", "кот на столе", "", "Текст: про кота\n\nКартинка: кот на столе"},
		{"", "кот на столе", "ru", "кот на столе"},
		{"про кота", "", "ru", "про кота"},
		{" ", "  ", "ru", ""},
	}
	for _, item := range cases {
		if got := joinExplanations(item.text, item.image, item.language); got != item.want {
			t.Errorf("joinExplanations(%q, %q, %q) = %q, want %q", item.text, item.image, item.language, got, item.want)
		}
	}
}

// summaryWorld is a fake Telegram and a fake llm around one processSummary call.
type summaryWorld struct {
	mu        sync.Mutex
	sent      []string
	edited    []string
	llmCalls  []string
	failImage bool
	failText  bool
	noFile    bool
}

func (world *summaryWorld) telegram(w http.ResponseWriter, r *http.Request) {
	world.mu.Lock()
	defer world.mu.Unlock()

	switch {
	case strings.HasSuffix(r.URL.Path, "/getFile"):
		if world.noFile {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"ok":false,"error_code":400,"description":"Bad Request: file is too big"}`)
			return
		}
		fmt.Fprint(w, `{"ok":true,"result":{"file_id":"p","file_path":"photos/p.jpg"}}`)
	case strings.Contains(r.URL.Path, "/file/"):
		fmt.Fprint(w, "image-bytes")
	case strings.HasSuffix(r.URL.Path, "/sendMessage"):
		world.sent = append(world.sent, r.FormValue("text"))
		fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d}}`, 100+len(world.sent))
	case strings.HasSuffix(r.URL.Path, "/editMessageText"):
		world.edited = append(world.edited, r.FormValue("text"))
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":101}}`)
	default:
		fmt.Fprint(w, `{"ok":true,"result":true}`)
	}
}

func (world *summaryWorld) llm(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	world.mu.Lock()
	defer world.mu.Unlock()

	isImage := strings.Contains(string(body), "image_url")
	if isImage {
		world.llmCalls = append(world.llmCalls, "image")
	} else {
		world.llmCalls = append(world.llmCalls, "text")
	}
	if (isImage && world.failImage) || (!isImage && world.failText) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	answer := "про текст"
	if isImage {
		answer = "про картинку"
	}
	fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, answer)
}

func runSummary(t *testing.T, world *summaryWorld, message *models.Message) {
	t.Helper()

	db, mock := database.ConnectToMockDB()
	model.Init(db)
	mock.MatchExpectationsInOrder(false)
	mock.ExpectQuery(`.*`).WillReturnRows(sqlmock.NewRows([]string{"id"}))

	telegram := httptest.NewServer(http.HandlerFunc(world.telegram))
	t.Cleanup(telegram.Close)
	llm := httptest.NewServer(http.HandlerFunc(world.llm))
	t.Cleanup(llm.Close)
	t.Setenv("AI_LLM_URL", llm.URL)

	b, err := bot.New("1:token", bot.WithServerURL(telegram.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}

	processSummary(context.Background(), b, &models.Update{Message: message})
}

func replyToPhoto(caption string) *models.Message {
	return &models.Message{
		ID:   2,
		Chat: models.Chat{ID: -100},
		From: &models.User{ID: 1, LanguageCode: "ru"},
		Text: "что тут",
		ReplyToMessage: &models.Message{
			ID:      1,
			Chat:    models.Chat{ID: -100},
			Caption: caption,
			Photo:   []models.PhotoSize{{FileID: "p", Width: 100, Height: 100}},
		},
	}
}

func lastSent(t *testing.T, world *summaryWorld) string {
	t.Helper()
	if len(world.sent) == 0 {
		t.Fatal("nothing was sent")
	}

	return world.sent[len(world.sent)-1]
}

func TestSummaryExplainsTextAndImageInOneMessage(t *testing.T) {
	world := &summaryWorld{}
	runSummary(t, world, replyToPhoto("Смотри какой кот"))

	// The wait message and the answer: exactly one answer.
	if len(world.sent) != 2 {
		t.Fatalf("want a wait message and one answer, got %q", world.sent)
	}
	if want := "Текст: про текст\n\nКартинка: про картинку"; lastSent(t, world) != want {
		t.Errorf("got %q, want %q", lastSent(t, world), want)
	}
}

func TestSummaryOfImageWithoutCaptionStaysAsItWas(t *testing.T) {
	world := &summaryWorld{}
	runSummary(t, world, replyToPhoto(""))

	if lastSent(t, world) != "про картинку" {
		t.Errorf("got %q", lastSent(t, world))
	}
	if len(world.llmCalls) != 1 {
		t.Errorf("want one llm call, got %v", world.llmCalls)
	}
}

func TestSummaryDoesNotExplainTheCommandInACaption(t *testing.T) {
	world := &summaryWorld{}
	runSummary(t, world, &models.Message{
		ID:      2,
		Chat:    models.Chat{ID: -100},
		From:    &models.User{ID: 1},
		Caption: "Что тут?",
		Photo:   []models.PhotoSize{{FileID: "p", Width: 100, Height: 100}},
	})

	if lastSent(t, world) != "про картинку" || len(world.llmCalls) != 1 {
		t.Errorf("got %q after %v", lastSent(t, world), world.llmCalls)
	}
}

func TestSummaryKeepsThePartThatWorked(t *testing.T) {
	world := &summaryWorld{failImage: true}
	runSummary(t, world, replyToPhoto("Смотри какой кот"))
	if lastSent(t, world) != "про текст" {
		t.Errorf("image failed: got %q", lastSent(t, world))
	}

	world = &summaryWorld{failText: true}
	runSummary(t, world, replyToPhoto("Смотри какой кот"))
	if lastSent(t, world) != "про картинку" {
		t.Errorf("text failed: got %q", lastSent(t, world))
	}

	world = &summaryWorld{noFile: true}
	runSummary(t, world, replyToPhoto("Смотри какой кот"))
	if lastSent(t, world) != "про текст" {
		t.Errorf("image too big: got %q", lastSent(t, world))
	}
}

func TestSummaryReportsWhenBothPartsFailed(t *testing.T) {
	world := &summaryWorld{failImage: true, failText: true}
	runSummary(t, world, replyToPhoto("Смотри какой кот"))
	if len(world.edited) != 1 || world.edited[0] != serverDeadText {
		t.Errorf("edited %q, sent %q", world.edited, world.sent)
	}

	world = &summaryWorld{noFile: true, failText: true}
	runSummary(t, world, replyToPhoto("Смотри какой кот"))
	if len(world.edited) != 1 || world.edited[0] != fileTooBigText {
		t.Errorf("edited %q, sent %q", world.edited, world.sent)
	}
}
