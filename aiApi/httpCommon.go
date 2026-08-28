package aiApi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
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

// pollResult делает один запрос к /api/result и разбирает ответ.
func pollResult(host, id string) (map[string]any, error) {
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

	var jsonRes map[string]any
	if err := json.Unmarshal(resBytes, &jsonRes); err != nil {
		return nil, fmt.Errorf("result is not valid json (%w): %s", err, string(resBytes))
	}

	return jsonRes, nil
}

// waitResult опрашивает /api/result, пока сервис не закончит генерацию,
// и возвращает раскодированный результат. kind участвует только в тексте
// ошибок и логов.
//
// maxWait отсчитывается по паузам между опросами, так что реальное ожидание
// выходит чуть длиннее на суммарное время самих запросов.
func waitResult(kind, host, id string, interval, maxWait time.Duration) ([]byte, error) {
	log.Printf("Start waiting for %s generation result\n", kind)

	maxAttempts := int(maxWait / interval)

	failures := 0
	for i := 0; ; i++ {
		if i == maxAttempts {
			return nil, fmt.Errorf("waiting for %s generation is longer than %s", kind, maxWait)
		}

		jsonRes, err := pollResult(host, id)
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

		status, ok := jsonRes["status"].(string)
		if !ok {
			return nil, errors.New("wrong response format while getting generation status")
		}

		if status == "pending" || status == "in_progress" {
			time.Sleep(interval)
			continue
		}
		if status == "error" {
			return nil, fmt.Errorf("error during %s generation", kind)
		}

		base64data, ok := jsonRes["data"].(string)
		if !ok {
			return nil, fmt.Errorf("wrong response format while getting %s data", kind)
		}

		return base64.StdEncoding.DecodeString(base64data)
	}
}
