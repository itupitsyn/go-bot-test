package aiApi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Сколько примерно занимает одна задача — из этого складывается ETA в очереди.
// Замер 2026-09-04 на боксе после починки памяти: видео в серии 85 с, видео
// следом за лёгкой задачей 111 с (перепиннинг весов стоит ~25 с), картинка
// 25 с. Округляем вверх: обещать дольше и отдать раньше лучше, чем наоборот.
const (
	videoJobEstimate = 100 * time.Second
	imageJobEstimate = 30 * time.Second
	// Транскрипция зависит от длины записи, так что это просто грубая середина.
	otherJobEstimate = 60 * time.Second
)

// QueueStatus — положение задачи в очереди сервиса генерации.
type QueueStatus struct {
	// Running — за задачу уже взялись, ждать осталось только её саму.
	Running bool
	// Ahead — сколько задач сервис обслужит раньше нашей, считая и ту, что
	// уже считается. Ноль означает, что очереди нет вовсе, и это важно
	// отличать от «одна впереди»: планировщик на сервисе не строго FIFO (он
	// добивает видео одного подтипа, пока модель тёплая), так что число —
	// это «сколько народу впереди», а не точный порядок обслуживания.
	Ahead int
	// ETA — грубая оценка времени до старта нашей задачи.
	ETA time.Duration
}

// ProgressFunc сообщает, где задача в очереди; вызывается на каждом опросе,
// пока та ждёт или считается.
type ProgressFunc func(QueueStatus)

// queueResponse — ответ /api/queue. Берём только то, из чего считается место:
// остальные поля там для диагностики сервиса.
type queueResponse struct {
	Running *struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		// Elapsed — сколько задача уже считается, в секундах.
		Elapsed float64 `json:"elapsed"`
	} `json:"running"`
	// Pending приходит по возрастанию времени постановки.
	Pending []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"pending"`
}

// jobEstimate — ожидаемая длительность задачи по её типу из ответа сервиса.
func jobEstimate(jobType string) time.Duration {
	switch jobType {
	case "t2v", "i2v":
		return videoJobEstimate
	case "img_gen":
		return imageJobEstimate
	default:
		return otherJobEstimate
	}
}

// getQueueStatus спрашивает у сервиса, где в очереди стоит задача id.
//
// Задачи нет ни на счёте, ни в очереди (уже готова, либо сервис перезапустился
// и забыл о ней) — возвращает nil без ошибки: показывать в этом случае нечего.
func getQueueStatus(host, id string) (*QueueStatus, error) {
	res, err := pollClient.Get(host + "/api/queue")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("queue request failed with status %d: %s", res.StatusCode, string(resBytes))
	}

	var parsed queueResponse
	if err := json.Unmarshal(resBytes, &parsed); err != nil {
		return nil, fmt.Errorf("queue is not valid json (%w): %s", err, string(resBytes))
	}

	if parsed.Running != nil && parsed.Running.ID == id {
		return &QueueStatus{Running: true}, nil
	}

	// До старта нашей задачи сервису надо доделать текущую и разгрести всё,
	// что встало в очередь раньше нас.
	var eta time.Duration
	ahead := 0

	if running := parsed.Running; running != nil {
		ahead++
		elapsed := time.Duration(running.Elapsed * float64(time.Second))
		if left := jobEstimate(running.Type) - elapsed; left > 0 {
			eta = left
		}
	}

	for _, job := range parsed.Pending {
		if job.ID == id {
			return &QueueStatus{Ahead: ahead, ETA: eta}, nil
		}
		ahead++
		eta += jobEstimate(job.Type)
	}

	return nil, nil
}
