package aiApi

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

// A live check of /api/queue parsing: it submits three images at once and looks
// at what getQueueStatus sees for each. It asserts nothing about the service
// numbers; it checks that we find our job in a real response at all.
//
//	AI_QUEUE_PROBE=1 go test ./aiApi/ -run TestQueueProbe -v -timeout 10m
//
// The host comes from AI_PAINTER_HOST; like the neighbouring probes, it is
// picked up from ../.env.local if not set in the environment.
func TestQueueProbe(t *testing.T) {
	if os.Getenv("AI_QUEUE_PROBE") == "" {
		t.Skip("AI_QUEUE_PROBE не задан")
	}

	if os.Getenv("AI_PAINTER_HOST") == "" {
		_ = godotenv.Load("../.env.local")
	}
	if os.Getenv("AI_PAINTER_HOST") == "" {
		t.Skip("AI_PAINTER_HOST не задан — пробнику некуда ходить")
	}

	var wg sync.WaitGroup

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := requestImage("кот на подоконнике", Caller{Progress: func(status QueueStatus) {
				t.Logf("задача %d: running=%v ahead=%d eta=%s",
					n, status.Running, status.Ahead, status.ETA.Round(time.Second))
			}}, promptOrigin{})
			if err != nil {
				t.Errorf("задача %d: %v", n, err)
			}
		}(i)
	}

	wg.Wait()
}
