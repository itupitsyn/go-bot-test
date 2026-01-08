package aiApi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand/v2"
	"net/http"
	"os"
	"strings"
)

func getT2VpromptId(prompt string) (error, string) {
	log.Println("Start getting prompt id")

	escaped, err := json.Marshal(prompt)
	if err != nil {
		return err, ""
	}

	jsonStr := strings.ReplaceAll(t2vPrompt, `"PositivePrompt"`, string(escaped))
	jsonStr = strings.ReplaceAll(jsonStr, "206275406212235", fmt.Sprint(rand.Int64N(math.MaxInt64)))
	url := fmt.Sprintf("%s/api/prompt", os.Getenv("AI_VIDEO_HOST"))

	resP, err := http.Post(url, "application/json", bytes.NewReader([]byte(jsonStr)))
	if err != nil {
		return err, ""
	}

	defer resP.Body.Close()

	resBytes, err := io.ReadAll(resP.Body)
	if err != nil {
		return err, ""
	}

	var promptJsonRes map[string]any
	err = json.Unmarshal(resBytes, &promptJsonRes)
	if err != nil {
		return err, ""
	}

	promptId, ok := promptJsonRes["prompt_id"].(string)
	if !ok {
		return errors.New("Error getting prompt_id"), ""
	}
	return nil, promptId
}

func generateT2V(prompt string) (error, []byte) {
	translatedPrompt, err := translatePrompt(prompt)

	if err != nil {
		return err, nil
	}

	err, promptId := getT2VpromptId(translatedPrompt)
	if err != nil {
		return err, nil
	}

	err = waitUntilGenerationFinished(promptId)
	if err != nil {
		return err, nil
	}

	err, filename := getGenerationResultFilename(promptId)
	if err != nil {
		return err, nil
	}

	err, video := getGenerationResult(*filename)

	return nil, video
}
