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

// http.DefaultClient has no timeout at all. If the generation service accepts
// the connection and goes silent, the goroutine hangs until the bot restarts,
// and so does the user who has already been told "Жди теперь".
const (
	// submitTimeout covers putting a job into the service queue. The response
	// with the id comes right away, but i2v uploads the image before that.
	submitTimeout = 2 * time.Minute
	// pollTimeout covers a single /api/result request, which answers instantly.
	pollTimeout = 30 * time.Second
	// llmTimeout covers the prompt translation.
	llmTimeout = time.Minute
	// maxPollFailures is how many failed polls in a row we tolerate. A single
	// timeout or 502 is no reason to abandon a job the service is still working
	// on.
	maxPollFailures = 3
)

// The clients share http.DefaultTransport, i.e. one connection pool, and differ
// only in the timeout.
var (
	submitClient = &http.Client{Timeout: submitTimeout}
	pollClient   = &http.Client{Timeout: pollTimeout}
	llmClient    = &http.Client{Timeout: llmTimeout}
)

// resultResponse is the /api/result response. The data field differs per job
// type: images and videos come as a base64 string, a transcription as a
// whisperx object, and with the error status it holds the error text. So it
// stays raw and is parsed by whoever submitted the job.
type resultResponse struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
}

// pollResult makes one /api/result request and parses the response.
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

// waitData polls /api/result until the service finishes and returns the data
// field unparsed. kind is used only in error and log messages.
//
// maxWait is counted by the pauses between polls, so the real wait comes out
// slightly longer by the total time of the requests themselves.
//
// onProgress, when set, receives the job's place in the queue on every poll.
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
			// On error the service puts its own text into data, which is far
			// more useful in the logs than "something went wrong".
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

// waitResult waits for a generation result and decodes it from base64.
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

// reportProgress tells the caller where the job is in the queue. A request
// error or a vanished job is swallowed silently: the place in the queue is
// decoration, and there is no point failing a generation that is going fine on
// the service because of it.
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

// checkSubmitStatus interprets the status code of a job submission.
//
// A refusal over the cap (429) is returned as a separate error: the service is
// not broken, it just won't take more from this person, and upstream that is
// shown in completely different words.
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
