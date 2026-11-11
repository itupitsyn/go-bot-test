package aiApi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// QueueStatus is the job's position in the generation service queue.
//
// Nothing here is computed by the bot. Both numbers come from the service,
// which is the only place that knows them: how the cards are split into lanes,
// which lane may take which kind of job, how many videos one machine is
// allowed at once, and how long jobs of this size have actually been taking
// lately.
//
// Until 2026-11-11 the bot worked them out itself, from constants, and was
// wrong twice over. It summed everything ahead as if one job followed another,
// while a video and a picture are served by different lanes of the same card
// and do not wait for each other at all; and it called every video 100 s when
// the service journal had them averaging 344-373 s, because people ask for
// long ones. A person was promised two minutes and waited six.
type QueueStatus struct {
	// Running means the job has been picked up, and only the job itself is
	// left to wait for.
	Running bool
	// Ahead is how many jobs will be served before ours, including the ones
	// already running, and counting only those that compete for the same lanes
	// — a video queue is not a queue for a picture.
	//
	// Zero means there is no queue at all, and it matters to tell that apart
	// from "one ahead": a free card does not pick the job up instantly, and in
	// that gap the bot must not announce a queue consisting of itself.
	Ahead int
	// ETA is how long until our job starts, as the service estimates it from
	// its own history.
	ETA time.Duration
}

// ProgressFunc reports where the job is in the queue; it is called on every
// poll while the job waits or runs.
type ProgressFunc func(QueueStatus)

// runningJob is the job a card is busy with.
type runningJob struct {
	ID string `json:"id"`
}

// queueResponse is the /api/queue response. Only what the place is read from
// is taken: the other fields are there for service diagnostics.
type queueResponse struct {
	// Running is the FIRST busy card only — a compatibility field from the
	// days of a single card. Kept for a service that reports no devices.
	Running *runningJob `json:"running"`
	// Devices is keyed by card, or by lane of a card: "node:index/lane". Our
	// own job has to be looked for in all of them, not in the field above,
	// which hides every busy card but one.
	Devices map[string]struct {
		Running *runningJob `json:"running"`
	} `json:"devices"`
	// Pending comes in order of SERVICE, not of submission: the order is set
	// by the round over people and by batching.
	Pending []struct {
		ID string `json:"id"`
		// Position and ETA are absent when the service has nobody to serve
		// this job with — then there is no honest number to show.
		Position *int     `json:"position"`
		ETA      *float64 `json:"eta"`
	} `json:"pending"`
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

	// Our job on the count: asked of EVERY card. With two lanes on one card a
	// picture is drawn by the second one while the first runs a video, and the
	// compatibility field shows only the video — so a job found in neither
	// place left the person looking at a stale "ты 2-й в очереди" over a
	// picture that was already being drawn.
	if parsed.Running != nil && parsed.Running.ID == id {
		return &QueueStatus{Running: true}, nil
	}
	for _, dev := range parsed.Devices {
		if dev.Running != nil && dev.Running.ID == id {
			return &QueueStatus{Running: true}, nil
		}
	}

	for _, job := range parsed.Pending {
		if job.ID != id {
			continue
		}

		// Queued, but the service named no numbers: show the plain wait rather
		// than invent them. The next poll is a few seconds away.
		status := QueueStatus{}
		if job.Position != nil {
			status.Ahead = *job.Position
		}
		if job.ETA != nil {
			status.ETA = time.Duration(*job.ETA * float64(time.Second))
		}

		return &status, nil
	}

	return nil, nil
}
