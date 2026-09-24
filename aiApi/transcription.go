package aiApi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	transcriptionPollInterval = 5 * time.Second
	transcriptionMaxWait      = time.Hour
)

// getTranscriptionId sends a file for recognition and returns the job id.
func getTranscriptionId(mediaBytes []byte, mediaName string, caller Caller) (string, error) {
	log.Println("Start getting transcription id")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// The fields go strictly before the file: WriteField creates a new part and
	// thereby closes the previous one, after which io.Copy into it fails with
	// "multipart: can't write to finished part". Same order as in
	// i2vGeneration.go.
	if user := caller.userForm(); user != "" {
		if err := writer.WriteField("user", user); err != nil {
			return "", err
		}
	}

	part, err := writer.CreateFormFile("file", mediaName)
	if err != nil {
		return "", err
	}
	if _, err = io.Copy(part, bytes.NewReader(mediaBytes)); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/api/transcription", os.Getenv("AI_TRANSCRIPTION_HOST"))
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	res, err := submitClient.Do(req)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	if err := checkSubmitStatus("transcription", res.StatusCode, resBytes); err != nil {
		return "", err
	}

	var jsonRes map[string]any
	if err := json.Unmarshal(resBytes, &jsonRes); err != nil {
		return "", fmt.Errorf("transcription response is not valid json (%w): %s", err, string(resBytes))
	}

	id, ok := jsonRes["id"].(string)
	if !ok {
		return "", errors.New("wrong response format while getting transcription id")
	}

	return id, nil
}

// transcriptionResult is what whisperx returns after alignment and diarization.
// There are more fields (timings, per-word splits), but the bot only needs the
// text and who said it.
type transcriptionResult struct {
	Segments []struct {
		Text    string `json:"text"`
		Speaker string `json:"speaker"`
	} `json:"segments"`
	Language string `json:"language"`
}

// formatTranscription glues segments into text. Consecutive lines of one person
// merge into a paragraph, and speaker labels appear only if the service heard
// more than one speaker: on a regular voice message they would be noise.
func formatTranscription(result transcriptionResult) string {
	speakers := make(map[string]int)
	for _, segment := range result.Segments {
		if segment.Speaker == "" {
			continue
		}
		if _, ok := speakers[segment.Speaker]; !ok {
			speakers[segment.Speaker] = len(speakers) + 1
		}
	}

	var text strings.Builder
	previousSpeaker := ""
	for i, segment := range result.Segments {
		segmentText := strings.TrimSpace(segment.Text)
		if segmentText == "" {
			continue
		}

		// The first segment opens the first paragraph; after that a paragraph
		// starts when the speaker changes.
		startsBlock := len(speakers) > 1 && (i == 0 || segment.Speaker != previousSpeaker)

		if text.Len() > 0 {
			if startsBlock {
				text.WriteString("\n\n")
			} else {
				text.WriteString(" ")
			}
		}
		if startsBlock {
			if number, ok := speakers[segment.Speaker]; ok {
				fmt.Fprintf(&text, "Говорящий %d: ", number)
			}
		}

		text.WriteString(segmentText)
		previousSpeaker = segment.Speaker
	}

	return text.String()
}

func generateTranscription(mediaBytes []byte, mediaName string, caller Caller) (string, error) {
	id, err := getTranscriptionId(mediaBytes, mediaName, caller)
	if err != nil {
		return "", err
	}

	data, err := waitData("transcription", os.Getenv("AI_TRANSCRIPTION_HOST"), id, transcriptionPollInterval, transcriptionMaxWait, caller.Progress)
	if err != nil {
		return "", err
	}

	var result transcriptionResult
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("transcription data is not a whisperx result (%w): %s", err, string(data))
	}

	return formatTranscription(result), nil
}
