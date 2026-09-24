package aiApi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Roughly how long one job takes; the queue ETA is built from this. Measured
// 2026-09-04 on the box after the memory fix: a video in a series takes 85 s, a
// video right after a light job 111 s (re-pinning the weights costs ~25 s), an
// image 25 s. Rounded up: promising longer and delivering sooner beats the
// opposite.
const (
	videoJobEstimate = 100 * time.Second
	imageJobEstimate = 30 * time.Second
	// Transcription depends on the recording length, so this is just a rough
	// middle.
	otherJobEstimate = 60 * time.Second
)

// QueueStatus is the job's position in the generation service queue.
type QueueStatus struct {
	// Running means the job has been picked up, and only the job itself is left
	// to wait for.
	Running bool
	// Ahead is how many jobs the service will serve before ours, including the
	// one already running. Zero means there is no queue at all, and it matters
	// to tell that apart from "one ahead": the scheduler on the service is not
	// strictly FIFO (it finishes videos of one subtype while the model is
	// warm), so the number means "how many people are ahead", not the exact
	// order of service.
	Ahead int
	// ETA is a rough estimate of the time until our job starts.
	ETA time.Duration
}

// ProgressFunc reports where the job is in the queue; it is called on every
// poll while the job waits or runs.
type ProgressFunc func(QueueStatus)

// queueResponse is the /api/queue response. Only what the place is computed
// from is taken: the other fields are there for service diagnostics.
type queueResponse struct {
	Running *struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		// Elapsed is how long the job has been running, in seconds.
		Elapsed float64 `json:"elapsed"`
	} `json:"running"`
	// Pending comes in ascending order of submission time.
	Pending []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"pending"`
}

// jobEstimate is the expected job duration by its type from the service
// response.
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

// getQueueStatus asks the service where job id stands in the queue.
//
// When the job is neither running nor queued (already done, or the service
// restarted and forgot about it), it returns nil without an error: there is
// nothing to show in that case.
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

	// Before our job starts, the service has to finish the current one and
	// clear everything queued before us.
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
