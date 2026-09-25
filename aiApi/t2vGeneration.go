package aiApi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
)

func getT2VId(prompt string, width, height, fps int, caller Caller, origin promptOrigin) (error, string) {
	log.Println("Start getting t2v id")

	escaped, err := json.Marshal(prompt)
	if err != nil {
		return err, ""
	}

	jsonStr := fmt.Sprintf(`{"prompt": %s, "width": %d, "height": %d, "fps": %d%s%s}`,
		string(escaped), width, height, fps, caller.userJSON(), origin.statsJSON())
	url := fmt.Sprintf("%s/api/t2v", os.Getenv("AI_VIDEO_HOST"))

	res, err := submitClient.Post(url, "application/json", bytes.NewReader([]byte(jsonStr)))
	if err != nil {
		return err, ""
	}

	defer res.Body.Close()

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return err, ""
	}

	if err := checkSubmitStatus("t2v", res.StatusCode, resBytes); err != nil {
		return err, ""
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

func generateT2V(prompt string, caller Caller) (error, []byte) {
	videoPrompt, origin, err := buildVideoPrompt(prompt)
	if err != nil {
		return err, nil
	}
	log.Printf("Video prompt: %s\n", videoPrompt)

	err, id := getT2VId(videoPrompt, defaultVideoWidth, defaultVideoHeight, defaultVideoFps, caller, origin)
	if err != nil {
		return err, nil
	}

	video, err := waitVideoResult(id, caller.Progress)
	if err != nil {
		return err, nil
	}

	return nil, video
}
