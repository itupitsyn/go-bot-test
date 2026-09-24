package bot

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"telebot/aiApi"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

func TestFormatQueueStatus(t *testing.T) {
	cases := []struct {
		name   string
		status aiApi.QueueStatus
		want   string
	}{
		{
			"за задачу взялись — очередь кончилась",
			aiApi.QueueStatus{Running: true},
			"Жди теперь",
		},
		{
			"очереди нет, задачу вот-вот возьмут",
			aiApi.QueueStatus{Ahead: 0},
			"Жди теперь",
		},
		{
			"карта занята чужим, больше никого",
			aiApi.QueueStatus{Ahead: 1, ETA: 80 * time.Second},
			"Ты 2-й в очереди, минут 1",
		},
		{
			"третий в очереди",
			aiApi.QueueStatus{Ahead: 2, ETA: 5 * time.Minute},
			"Ты 3-й в очереди, минут 5",
		},
		{
			"впереди затянувшаяся задача, оценка исчерпана",
			aiApi.QueueStatus{Ahead: 1, ETA: 3 * time.Second},
			"Ты 2-й в очереди, уже скоро",
		},
	}

	for _, c := range cases {
		if got := formatQueueStatus(c.status, waitText); got != c.want {
			t.Errorf("%s: получили %q, ждали %q", c.name, got, c.want)
		}
	}
}

// editRecorder records edits instead of going to Telegram.
type editRecorder struct {
	mu    sync.Mutex
	texts []string
	err   error
}

func (r *editRecorder) edit(text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.texts = append(r.texts, text)

	return r.err
}

func (r *editRecorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.texts...)
}

// testWaitMessage is a message with shortened pauses so the tests don't sleep.
func testWaitMessage() (*waitMessage, *editRecorder) {
	rec := &editRecorder{}
	w := newWaitMessage(1, waitText, okText, rec.edit)
	w.beat = 10 * time.Millisecond
	w.grace = 300 * time.Millisecond

	return w, rec
}

func expectTexts(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("правок %d, ждали %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("правка %d: %q, ждали %q", i, got[i], want[i])
		}
	}
}

// There is a queue: "Жди теперь" is not written at all, the place is shown
// right away.
func TestWaitMessageShowsQueueInsteadOfWaitPhrase(t *testing.T) {
	w, rec := testWaitMessage()

	go w.open()
	w.progress(aiApi.QueueStatus{Ahead: 2, ETA: 5 * time.Minute})
	<-w.opened

	expectTexts(t, rec.all(), []string{"Ты 3-й в очереди, минут 5"})
}

// There is no queue: everything as before, "Ладно", a pause, "Жди теперь".
func TestWaitMessageWithoutQueue(t *testing.T) {
	w, rec := testWaitMessage()

	go w.open()
	w.progress(aiApi.QueueStatus{Running: true})
	<-w.opened

	expectTexts(t, rec.all(), []string{waitText})
}

// The job hasn't been picked up yet, but nobody is ahead either: that is also
// "no queue", not a queue consisting of oneself.
func TestWaitMessageWithEmptyQueue(t *testing.T) {
	w, rec := testWaitMessage()

	go w.open()
	w.progress(aiApi.QueueStatus{Ahead: 0})
	<-w.opened

	expectTexts(t, rec.all(), []string{waitText})
}

// No news about the queue arrived in time (the service is thinking over the
// prompt): write "Жди теперь" blindly as before rather than stay silent.
func TestWaitMessageFallsBackWhenStatusIsLate(t *testing.T) {
	w, rec := testWaitMessage()

	go w.open()
	<-w.opened

	expectTexts(t, rec.all(), []string{waitText})
}

// The pause after "Ладно" stays even when the queue is known right away: it is
// there for the joke, not for technical reasons.
func TestWaitMessageKeepsTheBeat(t *testing.T) {
	w, rec := testWaitMessage()

	started := time.Now()
	go w.open()
	w.progress(aiApi.QueueStatus{Ahead: 2, ETA: time.Minute})
	<-w.opened

	if elapsed := time.Since(started); elapsed < w.beat {
		t.Errorf("вступление отыграло за %s, пауза %s", elapsed, w.beat)
	}
	expectTexts(t, rec.all(), []string{"Ты 3-й в очереди, минут 1"})
}

// The queue has reached our job: only then does "Жди теперь" appear.
func TestWaitMessageSwitchesToWaitPhraseWhenQueueEnds(t *testing.T) {
	w, rec := testWaitMessage()

	go w.open()
	w.progress(aiApi.QueueStatus{Ahead: 2, ETA: 3 * time.Minute})
	<-w.opened
	w.progress(aiApi.QueueStatus{Ahead: 1, ETA: time.Minute})
	w.progress(aiApi.QueueStatus{Ahead: 1, ETA: time.Minute})
	w.progress(aiApi.QueueStatus{Running: true})

	expectTexts(t, rec.all(), []string{
		"Ты 3-й в очереди, минут 3",
		"Ты 2-й в очереди, минут 1",
		waitText,
	})
}

// The error appears before the pause runs out: the opening must not overwrite
// it.
func TestWaitMessageDoneStopsTheOpening(t *testing.T) {
	w, rec := testWaitMessage()

	go w.open()
	w.done()
	time.Sleep(w.grace + 50*time.Millisecond)

	expectTexts(t, rec.all(), nil)
}

// Telegram refused to edit the message: the next text change tries again.
func TestWaitMessageSurvivesEditError(t *testing.T) {
	w, rec := testWaitMessage()
	rec.err = errors.New("telegram is unhappy")

	go w.open()
	w.progress(aiApi.QueueStatus{Ahead: 2, ETA: 3 * time.Minute})
	<-w.opened
	w.progress(aiApi.QueueStatus{Running: true})

	expectTexts(t, rec.all(), []string{"Ты 3-й в очереди, минут 3", waitText})
}

// On nil (sending "Ладно" failed) the methods don't crash.
func TestWaitMessageToleratesNil(t *testing.T) {
	var w *waitMessage

	w.progress(aiApi.QueueStatus{Ahead: 1})
	w.done()

	if w.id() != 0 {
		t.Error("у несуществующего сообщения появился номер")
	}
}

// testInlineWaitMessage is an inline message with shortened pauses.
func testInlineWaitMessage() (*waitMessage, *editRecorder) {
	rec := &editRecorder{}
	w := newWaitMessage(0, inlineWaitText, inlineOkText, rec.edit)
	w.beat = 10 * time.Millisecond
	w.grace = 300 * time.Millisecond

	return w, rec
}

// The pause after the opening in inline is the same as in a chat and holds even
// when the queue is known right away.
func TestInlineWaitMessageKeepsTheBeat(t *testing.T) {
	w, rec := testInlineWaitMessage()

	started := time.Now()
	go w.open()
	w.progress(aiApi.QueueStatus{Ahead: 2, ETA: 5 * time.Minute})
	<-w.opened

	if elapsed := time.Since(started); elapsed < w.beat {
		t.Errorf("вступление отыграло за %s, пауза %s", elapsed, w.beat)
	}
	expectTexts(t, rec.all(), []string{"Ты 3-й в очереди, минут 5"})
}

// Inline follows the same order as a chat, only with its own words: the opening
// is already written, then the queue, and "ОЖИДАЕМ!!!" comes last.
func TestInlineWaitMessageShowsQueueThenWaitPhrase(t *testing.T) {
	w, rec := testInlineWaitMessage()

	go w.open()
	w.progress(aiApi.QueueStatus{Ahead: 2, ETA: 5 * time.Minute})
	<-w.opened
	w.progress(aiApi.QueueStatus{Running: true})

	expectTexts(t, rec.all(), []string{"Ты 3-й в очереди, минут 5", inlineWaitText})
}

// No queue: "ОЖИДАЕМ!!!" right away, and there must be no "Жди теперь" in
// inline.
func TestInlineWaitMessageWithoutQueue(t *testing.T) {
	w, rec := testInlineWaitMessage()

	go w.open()
	w.progress(aiApi.QueueStatus{Ahead: 0})
	<-w.opened

	expectTexts(t, rec.all(), []string{inlineWaitText})

	for _, text := range rec.all() {
		if strings.Contains(text, waitText) {
			t.Errorf("в inline-сообщение попало %q: %q", waitText, text)
		}
	}
}

func TestGenerationErrorText(t *testing.T) {
	// A refusal over the cap is not a failure, and a dead server must not be
	// mentioned
	if got := generationErrorText(fmt.Errorf("t2v: %w", aiApi.ErrQueueFull)); got != queueFullText {
		t.Errorf("на переполненную очередь получили %q, ждали %q", got, queueFullText)
	}

	if got := generationErrorText(errors.New("connection refused")); got != serverDeadText {
		t.Errorf("на поломку получили %q, ждали %q", got, serverDeadText)
	}
}

func TestMessageCallerTakesTheSender(t *testing.T) {
	w, _ := testWaitMessage()

	caller := messageCaller(&models.Message{From: &models.User{ID: 77}}, w)
	if caller.UserID != 77 {
		t.Errorf("UserID = %d, ждали 77", caller.UserID)
	}
	if caller.Progress == nil {
		t.Error("колбэк очереди потерялся")
	}

	// A post on behalf of a channel: there is no sender, so no id is made up
	if got := messageCaller(&models.Message{}, w).UserID; got != 0 {
		t.Errorf("без отправителя UserID = %d, ждали 0", got)
	}
}
