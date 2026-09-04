package aiApi

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

// Живая проверка разбора /api/queue: ставит три картинки разом и смотрит, что
// getQueueStatus видит по каждой. Ничего не утверждает про числа сервиса —
// проверяет, что мы вообще находим свою задачу в реальном ответе.
//
//	AI_QUEUE_PROBE=1 go test ./aiApi/ -run TestQueueProbe -v -timeout 10m
//
// Хост берётся из AI_PAINTER_HOST — как и соседние пробники, подхватываем
// его из ../.env.local, если в окружении не задан.
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
			_, err := requestImage("кот на подоконнике", func(status QueueStatus) {
				t.Logf("задача %d: running=%v ahead=%d eta=%s",
					n, status.Running, status.Ahead, status.ETA.Round(time.Second))
			})
			if err != nil {
				t.Errorf("задача %d: %v", n, err)
			}
		}(i)
	}

	wg.Wait()
}
