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

func TestGetQueueStatusCountsAhead(t *testing.T) {
	// Someone else's video is running (12 s of ~100), with another video and an
	// image ahead of us.
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
	// Two queued ahead of us plus the one already running.
	if status.Ahead != 3 {
		t.Errorf("впереди %d задач, ждали 3", status.Ahead)
	}

	want := (videoJobEstimate - 12*time.Second) + videoJobEstimate + imageJobEstimate
	if status.ETA != want {
		t.Errorf("ETA %s, ждали %s", status.ETA, want)
	}
}

// A free GPU does not pick up a job instantly: in that gap the job is already
// queued, but nobody is ahead, so there is no queue.
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

// The job has already finished (it is neither running nor queued): not an
// error, just nothing to show.
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

// An overrunning job must not yield a negative remainder.
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
	// An overrunning job is still ahead of us even when its estimate is used
	// up.
	if status.Ahead != 1 {
		t.Errorf("впереди %d задач, ждали 1", status.Ahead)
	}
}
