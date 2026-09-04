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

// editRecorder запоминает правки вместо похода в Telegram.
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

// testWaitMessage — сообщение с укороченными паузами, чтобы тесты не спали.
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

// Очередь есть — «Жди теперь» не пишем вовсе, сразу говорим место.
func TestWaitMessageShowsQueueInsteadOfWaitPhrase(t *testing.T) {
	w, rec := testWaitMessage()

	go w.open()
	w.progress(aiApi.QueueStatus{Ahead: 2, ETA: 5 * time.Minute})
	<-w.opened

	expectTexts(t, rec.all(), []string{"Ты 3-й в очереди, минут 5"})
}

// Очереди нет — всё как раньше: «Ладно», пауза, «Жди теперь».
func TestWaitMessageWithoutQueue(t *testing.T) {
	w, rec := testWaitMessage()

	go w.open()
	w.progress(aiApi.QueueStatus{Running: true})
	<-w.opened

	expectTexts(t, rec.all(), []string{waitText})
}

// Задачу ещё не успели взять на счёт, но и впереди никого — это тоже «нет
// очереди», а не очередь из одного себя.
func TestWaitMessageWithEmptyQueue(t *testing.T) {
	w, rec := testWaitMessage()

	go w.open()
	w.progress(aiApi.QueueStatus{Ahead: 0})
	<-w.opened

	expectTexts(t, rec.all(), []string{waitText})
}

// Вестей об очереди не дождались (сервис думает над промптом) — пишем
// «Жди теперь» вслепую, как раньше, а не молчим.
func TestWaitMessageFallsBackWhenStatusIsLate(t *testing.T) {
	w, rec := testWaitMessage()

	go w.open()
	<-w.opened

	expectTexts(t, rec.all(), []string{waitText})
}

// Пауза после «Ладно» остаётся, даже когда про очередь известно сразу:
// она тут ради шутки, а не ради техники.
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

// Очередь дошла до нашей задачи — только тогда появляется «Жди теперь».
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

// Ошибка появляется раньше, чем истечёт пауза, — вступление не должно её
// затереть.
func TestWaitMessageDoneStopsTheOpening(t *testing.T) {
	w, rec := testWaitMessage()

	go w.open()
	w.done()
	time.Sleep(w.grace + 50*time.Millisecond)

	expectTexts(t, rec.all(), nil)
}

// Телеграм не дал поправить сообщение — следующая смена текста снова пробует.
func TestWaitMessageSurvivesEditError(t *testing.T) {
	w, rec := testWaitMessage()
	rec.err = errors.New("telegram is unhappy")

	go w.open()
	w.progress(aiApi.QueueStatus{Ahead: 2, ETA: 3 * time.Minute})
	<-w.opened
	w.progress(aiApi.QueueStatus{Running: true})

	expectTexts(t, rec.all(), []string{"Ты 3-й в очереди, минут 3", waitText})
}

// На nil (не удалось отправить «Ладно») методы не падают.
func TestWaitMessageToleratesNil(t *testing.T) {
	var w *waitMessage

	w.progress(aiApi.QueueStatus{Ahead: 1})
	w.done()

	if w.id() != 0 {
		t.Error("у несуществующего сообщения появился номер")
	}
}

// testInlineWaitMessage — inline-сообщение с укороченными паузами.
func testInlineWaitMessage() (*waitMessage, *editRecorder) {
	rec := &editRecorder{}
	w := newWaitMessage(0, inlineWaitText, inlineOkText, rec.edit)
	w.beat = 10 * time.Millisecond
	w.grace = 300 * time.Millisecond

	return w, rec
}

// Пауза после вступления в inline такая же, как в чате, и держится, даже
// когда про очередь известно сразу.
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

// В inline тот же порядок, что и в чате, только слова свои: вступление уже
// написано, дальше очередь, а «ОЖИДАЕМ!!!» — последнее.
func TestInlineWaitMessageShowsQueueThenWaitPhrase(t *testing.T) {
	w, rec := testInlineWaitMessage()

	go w.open()
	w.progress(aiApi.QueueStatus{Ahead: 2, ETA: 5 * time.Minute})
	<-w.opened
	w.progress(aiApi.QueueStatus{Running: true})

	expectTexts(t, rec.all(), []string{"Ты 3-й в очереди, минут 5", inlineWaitText})
}

// Очереди нет — сразу «ОЖИДАЕМ!!!», и никакого «Жди теперь» в inline быть
// не должно.
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
	// Отказ по потолку — не поломка, и говорить про подохший сервер нельзя
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

	// Пост от имени канала: отправителя нет — id не выдумываем
	if got := messageCaller(&models.Message{}, w).UserID; got != 0 {
		t.Errorf("без отправителя UserID = %d, ждали 0", got)
	}
}
