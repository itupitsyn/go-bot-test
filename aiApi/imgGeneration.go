package aiApi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

const (
	imagePollInterval = 5 * time.Second
	imageMaxWait      = time.Hour
)

// imageCommandPrefixes are the ways a request opens. Cut off before anything
// else: they address the bot, they are not part of the picture.
var imageCommandPrefixes = []string{"draw ", "нарисуй "}

// getImagePrompt strips the command word and leaves the rest ALONE.
//
// It used to lowercase the whole message, because the style keywords were
// matched by suffix and matching had to be case-blind. The keywords are gone
// (see promptEnhancer.go), and the lowercasing turned out to cost something:
// the picture model draws the letters it is given, so "ВЫВЕСКА «КОФЕ»" was
// reaching it as "вывеска «кофе»" and the sign came out in lower case. Only
// the prefix is matched case-blind now; everything the person typed goes on
// as typed.
func getImagePrompt(msgText string) string {
	text := strings.TrimSpace(msgText)

	for _, prefix := range imageCommandPrefixes {
		if len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix) {
			return strings.TrimSpace(text[len(prefix):])
		}
	}

	return text
}

func generateImage(msgText string, caller Caller) ([]byte, error) {
	prompt := getImagePrompt(msgText)
	log.Printf("Got prompt %s\n", prompt)

	finalPrompt, mode, err := buildImagePrompt(prompt)
	if err != nil {
		return nil, err
	}

	log.Printf("Image prompt (%s): %s\n", mode, finalPrompt)

	return requestImage(finalPrompt, caller,
		promptOrigin{Source: msgText, Style: mode})
}

// requestImage sends an already assembled prompt and waits for the finished
// image.
func requestImage(prompt string, caller Caller, origin promptOrigin) ([]byte, error) {
	escapedPrompt, err := json.Marshal(prompt)
	if err != nil {
		return nil, err
	}

	jsonStr := fmt.Appendf(nil, `{"prompt": %s%s%s%s}`, string(escapedPrompt),
		caller.userJSON(), caller.chatJSON(), origin.statsJSON())

	url := fmt.Sprintf("%s/api/txt2img", os.Getenv("AI_PAINTER_HOST"))
	res, err := submitClient.Post(url, "application/json", bytes.NewReader(jsonStr))
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if err := checkSubmitStatus("image", res.StatusCode, resBytes); err != nil {
		return nil, err
	}

	var jsonRes map[string]any
	err = json.Unmarshal(resBytes, &jsonRes)
	if err != nil {
		return nil, err
	}

	id, ok := jsonRes["id"].(string)
	if !ok {
		return nil, errors.New("wrong response format while getting id")
	}

	caller.jobNamed(id)

	return waitResult("image", os.Getenv("AI_PAINTER_HOST"), id, imagePollInterval, imageMaxWait, caller.Progress)
}
