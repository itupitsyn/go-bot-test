package aiApi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	defaultVideoWidth  = 832
	defaultVideoHeight = 480
	defaultVideoFps    = 30
)

// waitVideoResult polls the video service /api/result endpoint until the
// generation is finished and returns the decoded video bytes.
func waitVideoResult(id string) ([]byte, error) {
	log.Println("Start waiting for video generation result")

	var jsonRes map[string]any
	for i := 0; ; i++ {
		if i == 200 {
			return nil, errors.New("Waiting for video generation is too long")
		}

		url := fmt.Sprintf("%s/api/result", os.Getenv("AI_VIDEO_HOST"))
		res, err := http.Get(fmt.Sprintf("%s?id=%s", url, id))
		if err != nil {
			return nil, err
		}

		resBytes, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return nil, err
		}

		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("video result request failed with status %d: %s", res.StatusCode, string(resBytes))
		}

		err = json.Unmarshal(resBytes, &jsonRes)
		if err != nil {
			return nil, fmt.Errorf("video result is not valid json (%w): %s", err, string(resBytes))
		}

		status, ok := jsonRes["status"].(string)
		if !ok {
			return nil, errors.New("wrong response format while getting generation status")
		}

		if status == "pending" || status == "in_progress" {
			time.Sleep(10 * time.Second)
			continue
		} else if status == "error" {
			return nil, errors.New("error during video generation")
		}

		break
	}

	base64video, ok := jsonRes["data"].(string)
	if !ok {
		return nil, errors.New("wrong response format while getting video data")
	}

	return base64.StdEncoding.DecodeString(base64video)
}
