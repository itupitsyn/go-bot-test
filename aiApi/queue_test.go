package aiApi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// queueServer starts a stub /api/queue with a canned response body.
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

// Our picture is being drawn by the diffusers lane while the comfy lane runs
// someone's video. The compatibility "running" field shows only the first busy
// card, so finding ourselves means asking every card.
func TestGetQueueStatusRunningOnSecondLane(t *testing.T) {
	host := queueServer(t, `{
		"running": {"id": "other-video", "type": "t2v", "elapsed": 20.0},
		"devices": {
			"gen-main:0/comfy": {"running": {"id": "other-video", "type": "t2v", "elapsed": 20.0}},
			"gen-main:0/diffusers": {"running": {"id": "our", "type": "img_gen", "elapsed": 5.0}},
			"llm-box:0": {"running": null}
		},
		"pending": []
	}`)

	status, err := getQueueStatus(host, "our")
	if err != nil {
		t.Fatal(err)
	}
	if status == nil || !status.Running {
		t.Fatalf("свою задачу на второй полосе не узнали: %+v", status)
	}
}

// The place and the wait are read as the service gives them: it is the only
// side that knows the lanes, the ceilings and how long such jobs really take.
func TestGetQueueStatusTakesThePlaceFromTheService(t *testing.T) {
	host := queueServer(t, `{
		"running": {"id": "other", "type": "t2v", "elapsed": 12.0},
		"devices": {"gen-main:0/comfy": {"running": {"id": "other", "type": "t2v", "elapsed": 12.0}}},
		"pending": [
			{"id": "first", "type": "t2v", "position": 1, "eta": 361.0},
			{"id": "our", "type": "t2v", "position": 2, "eta": 734.5}
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
	if status.Ahead != 2 {
		t.Errorf("впереди %d задач, ждали 2", status.Ahead)
	}

	want := 734500 * time.Millisecond
	if status.ETA != want {
		t.Errorf("ETA %s, ждали %s", status.ETA, want)
	}
}

// A video queue is no queue for a picture: the lanes are served independently,
// and the service says so with zeros. The bot must not add anything of its
// own on top — the videos ahead are plainly visible in the same response.
func TestGetQueueStatusTrustsZerosOverVisibleVideos(t *testing.T) {
	host := queueServer(t, `{
		"running": {"id": "video-1", "type": "t2v", "elapsed": 10.0},
		"devices": {
			"gen-main:0/comfy": {"running": {"id": "video-1", "type": "t2v", "elapsed": 10.0}},
			"gen-main:0/diffusers": {"running": null}
		},
		"pending": [
			{"id": "video-2", "type": "t2v", "position": 1, "eta": 350.0},
			{"id": "our", "type": "img_gen", "position": 0, "eta": 0.0}
		]
	}`)

	status, err := getQueueStatus(host, "our")
	if err != nil {
		t.Fatal(err)
	}
	if status == nil {
		t.Fatal("задачу в очереди не нашли")
	}
	if status.Ahead != 0 || status.ETA != 0 {
		t.Errorf("картинка за роликами: ahead=%d eta=%s, ждали нули",
			status.Ahead, status.ETA)
	}
}

// A free GPU does not pick up a job instantly: in that gap the job is already
// queued, but nobody is ahead, so there is no queue.
func TestGetQueueStatusEmptyQueue(t *testing.T) {
	host := queueServer(t, `{"running": null, "devices": {},
		"pending": [{"id": "our", "type": "img_gen", "position": 0, "eta": 0.0}]}`)

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

// The service named no numbers (nobody to serve this kind of job, or an older
// service that does not count them). Queued is still queued: show the plain
// wait instead of inventing a place.
func TestGetQueueStatusWithoutNumbersIsAPlainWait(t *testing.T) {
	host := queueServer(t, `{"running": null, "devices": {},
		"pending": [{"id": "our", "type": "img_gen"}]}`)

	status, err := getQueueStatus(host, "our")
	if err != nil {
		t.Fatal(err)
	}
	if status == nil {
		t.Fatal("задача в очереди есть, статус должен быть")
	}
	if status.Running || status.Ahead != 0 || status.ETA != 0 {
		t.Errorf("без чисел от сервиса ждали пустой статус, получили %+v", status)
	}
}

// The job has already finished (it is neither running nor queued): not an
// error, just nothing to show.
func TestGetQueueStatusUnknownJob(t *testing.T) {
	host := queueServer(t, `{"running": null, "devices": {}, "pending": []}`)

	status, err := getQueueStatus(host, "our")
	if err != nil {
		t.Fatal(err)
	}
	if status != nil {
		t.Errorf("для неизвестной задачи вернулся статус %+v", status)
	}
}
