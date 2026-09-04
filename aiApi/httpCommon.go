package aiApi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// У http.DefaultClient таймаут не выставлен вообще. Если сервис генерации
// принял соединение и замолчал, горутина висит до рестарта бота, а вместе с
// ней и пользователь, которому уже ушло "Жди теперь".
const (
	// submitTimeout — постановка задачи в очередь сервиса. Ответ с id
	// приходит сразу, но i2v перед этим успевает залить картинку.
	submitTimeout = 2 * time.Minute
	// pollTimeout — один запрос /api/result, он отвечает мгновенно.
	pollTimeout = 30 * time.Second
	// llmTimeout — перевод промпта.
	llmTimeout = time.Minute
	// maxPollFailures — сколько неудачных опросов подряд терпим. Одиночный
	// таймаут или 502 не повод бросать задачу, которая на сервисе всё ещё
	// считается.
	maxPollFailures = 3
)

// Клиенты делят http.DefaultTransport, то есть общий пул соединений,
// и различаются только таймаутом.
var (
	submitClient = &http.Client{Timeout: submitTimeout}
	pollClient   = &http.Client{Timeout: pollTimeout}
	llmClient    = &http.Client{Timeout: llmTimeout}
)

// resultResponse — ответ /api/result. Поле data у каждого типа задачи своё:
// картинки и видео приходят base64-строкой, расшифровка — объектом whisperx,
// а у статуса error там текст ошибки. Поэтому оно остаётся сырым, и разбирает
// его тот, кто задачу ставил.
type resultResponse struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
}

// pollResult делает один запрос к /api/result и разбирает ответ.
func pollResult(host, id string) (*resultResponse, error) {
	res, err := pollClient.Get(fmt.Sprintf("%s/api/result?id=%s", host, id))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("result request failed with status %d: %s", res.StatusCode, string(resBytes))
	}

	var parsed resultResponse
	if err := json.Unmarshal(resBytes, &parsed); err != nil {
		return nil, fmt.Errorf("result is not valid json (%w): %s", err, string(resBytes))
	}

	return &parsed, nil
}

// waitData опрашивает /api/result, пока сервис не закончит работу, и
// возвращает поле data неразобранным. kind участвует только в тексте ошибок и
// логов.
//
// maxWait отсчитывается по паузам между опросами, так что реальное ожидание
// выходит чуть длиннее на суммарное время самих запросов.
//
// onProgress, если задан, получает место задачи в очереди на каждом опросе.
func waitData(kind, host, id string, interval, maxWait time.Duration, onProgress ProgressFunc) (json.RawMessage, error) {
	log.Printf("Start waiting for %s result\n", kind)

	maxAttempts := int(maxWait / interval)

	failures := 0
	for i := 0; ; i++ {
		if i == maxAttempts {
			return nil, fmt.Errorf("waiting for %s is longer than %s", kind, maxWait)
		}

		res, err := pollResult(host, id)
		if err != nil {
			failures++
			if failures >= maxPollFailures {
				return nil, fmt.Errorf("%s result request failed %d times in a row, last error: %w", kind, failures, err)
			}
			log.Printf("[warn] %s result request failed (%d/%d): %v\n", kind, failures, maxPollFailures, err)
			time.Sleep(interval)
			continue
		}
		failures = 0

		switch res.Status {
		case "pending", "in_progress":
			reportProgress(host, id, onProgress)
			time.Sleep(interval)
			continue
		case "error":
			// При ошибке сервис кладёт в data свой текст — он куда полезнее
			// в логах, чем "что-то пошло не так".
			var message string
			if err := json.Unmarshal(res.Data, &message); err != nil {
				message = string(res.Data)
			}
			return nil, fmt.Errorf("error during %s: %s", kind, message)
		case "done":
			if len(res.Data) == 0 {
				return nil, fmt.Errorf("%s finished without any data", kind)
			}
			return res.Data, nil
		default:
			return nil, fmt.Errorf("unknown %s status %q", kind, res.Status)
		}
	}
}

// waitResult ждёт результат генерации и раскодирует его из base64.
func waitResult(kind, host, id string, interval, maxWait time.Duration, onProgress ProgressFunc) ([]byte, error) {
	data, err := waitData(kind+" generation", host, id, interval, maxWait, onProgress)
	if err != nil {
		return nil, err
	}

	var base64data string
	if err := json.Unmarshal(data, &base64data); err != nil {
		return nil, fmt.Errorf("%s data is not a base64 string (%w)", kind, err)
	}

	return base64.StdEncoding.DecodeString(base64data)
}

// reportProgress сообщает вызывающему, где задача в очереди. Ошибку запроса и
// пропавшую задачу глотаем молча: место в очереди — украшение, ронять из-за
// него генерацию, которая на сервисе идёт нормально, незачем.
func reportProgress(host, id string, onProgress ProgressFunc) {
	if onProgress == nil {
		return
	}

	status, err := getQueueStatus(host, id)
	if err != nil || status == nil {
		return
	}

	onProgress(*status)
}

// checkSubmitStatus разбирает код ответа на постановку задачи.
//
// Отказ по потолку (429) отдаём отдельной ошибкой: сервис не сломался, он
// просто не берёт у этого человека больше, и наверху это показывается совсем
// другими словами.
func checkSubmitStatus(kind string, statusCode int, body []byte) error {
	switch statusCode {
	case http.StatusOK:
		return nil
	case http.StatusTooManyRequests:
		return fmt.Errorf("%s: %w", kind, ErrQueueFull)
	default:
		return fmt.Errorf("%s request failed with status %d: %s", kind, statusCode, string(body))
	}
}
