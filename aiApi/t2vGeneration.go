package aiApi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func getT2VId(prompt string, width, height, fps int) (error, string) {
	log.Println("Start getting t2v id")

	escaped, err := json.Marshal(prompt)
	if err != nil {
		return err, ""
	}

	jsonStr := fmt.Sprintf(`{"prompt": %s, "width": %d, "height": %d, "fps": %d}`, string(escaped), width, height, fps)
	url := fmt.Sprintf("%s/api/t2v", os.Getenv("AI_VIDEO_HOST"))

	res, err := http.Post(url, "application/json", bytes.NewReader([]byte(jsonStr)))
	if err != nil {
		return err, ""
	}

	defer res.Body.Close()

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return err, ""
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("t2v request failed with status %d: %s", res.StatusCode, string(resBytes)), ""
	}

	var jsonRes map[string]any
	err = json.Unmarshal(resBytes, &jsonRes)
	if err != nil {
		return fmt.Errorf("t2v response is not valid json (%w): %s", err, string(resBytes)), ""
	}

	id, ok := jsonRes["id"].(string)
	if !ok {
		return errors.New("wrong response format while getting t2v id"), ""
	}

	return nil, id
}

func generateT2V(prompt string) (error, []byte) {
	translatedPrompt, err := translatePrompt(prompt)
	if err != nil {
		return err, nil
	}

	err, id := getT2VId(translatedPrompt, defaultVideoWidth, defaultVideoHeight, defaultVideoFps)
	if err != nil {
		return err, nil
	}

	video, err := waitVideoResult(id)
	if err != nil {
		return err, nil
	}

	return nil, video
}
