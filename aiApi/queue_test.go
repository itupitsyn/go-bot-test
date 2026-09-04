package aiApi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// queueServer поднимает заглушку /api/queue с готовым телом ответа.
func queueServer(t *testing.T, body string) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/queue" {
			t.Errorf("запрос ушёл не туда: %s", r.URL.Path)
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)

	return server.URL
}

func TestGetQueueStatusRunning(t *testing.T) {
	host := queueServer(t, `{"running": {"id": "our", "type": "t2v", "elapsed": 12.0}, "pending": []}`)

	status, err := getQueueStatus(host, "our")
	if err != nil {
		t.Fatal(err)
	}
	if status == nil || !status.Running {
		t.Fatalf("свою задачу на счёте не узнали: %+v", status)
	}
}

func TestGetQueueStatusCountsAhead(t *testing.T) {
	// Считается чужое видео (12 с из ~100), перед нами ещё видео и картинка.
	host := queueServer(t, `{
		"running": {"id": "other", "type": "i2v", "elapsed": 12.0},
		"pending": [
			{"id": "first", "type": "t2v"},
			{"id": "second", "type": "img_gen"},
			{"id": "our", "type": "t2v"}
		]
	}`)

	status, err := getQueueStatus(host, "our")
	if err != nil {
		t.Fatal(err)
	}
	if status == nil {
		t.Fatal("задачу в очереди не нашли")
	}
	if status.Running {
		t.Error("наша задача ещё ждёт, а помечена как считающаяся")
	}
	// Двое в очереди перед нами плюс та, что уже считается.
	if status.Ahead != 3 {
		t.Errorf("впереди %d задач, ждали 3", status.Ahead)
	}

	want := (videoJobEstimate - 12*time.Second) + videoJobEstimate + imageJobEstimate
	if status.ETA != want {
		t.Errorf("ETA %s, ждали %s", status.ETA, want)
	}
}

// Свободная карта берёт задачу не мгновенно: в этот зазор задача уже в
// очереди, а впереди никого — очереди нет.
func TestGetQueueStatusEmptyQueue(t *testing.T) {
	host := queueServer(t, `{"running": null, "pending": [{"id": "our", "type": "img_gen"}]}`)

	status, err := getQueueStatus(host, "our")
	if err != nil {
		t.Fatal(err)
	}
	if status == nil {
		t.Fatal("задачу в очереди не нашли")
	}
	if status.Ahead != 0 || status.ETA != 0 {
		t.Errorf("на пустой очереди ahead=%d eta=%s, ждали нули", status.Ahead, status.ETA)
	}
}

// Задача уже досчиталась (её нет ни на счёте, ни в очереди) — это не ошибка,
// просто показывать нечего.
func TestGetQueueStatusUnknownJob(t *testing.T) {
	host := queueServer(t, `{"running": null, "pending": []}`)

	status, err := getQueueStatus(host, "our")
	if err != nil {
		t.Fatal(err)
	}
	if status != nil {
		t.Errorf("для неизвестной задачи вернулся статус %+v", status)
	}
}

// Затянувшаяся задача не должна давать отрицательный остаток.
func TestGetQueueStatusOverdueRunningJob(t *testing.T) {
	host := queueServer(t, `{
		"running": {"id": "other", "type": "t2v", "elapsed": 900.0},
		"pending": [{"id": "our", "type": "t2v"}]
	}`)

	status, err := getQueueStatus(host, "our")
	if err != nil {
		t.Fatal(err)
	}
	if status == nil {
		t.Fatal("задачу в очереди не нашли")
	}
	if status.ETA != 0 {
		t.Errorf("ETA %s, ждали 0", status.ETA)
	}
	// Затянувшаяся задача всё ещё впереди нас, даже когда оценка исчерпана.
	if status.Ahead != 1 {
		t.Errorf("впереди %d задач, ждали 1", status.Ahead)
	}
}
