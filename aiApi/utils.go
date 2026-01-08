package aiApi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"os"
	"time"
)

type GenerationResult struct {
	filename  string
	subfolder string
}

func waitUntilGenerationFinished(promptId string) error {
	log.Println("Start I2V generation")
	isFinished := false

	i := 0
	for !isFinished {
		if i == 200 {
			return errors.New("Waiting for I2V generation is too long")
		}

		url := fmt.Sprintf("%s/api/queue", os.Getenv("AI_VIDEO_HOST"))
		resQ, err := http.Get(url)
		if err != nil {
			return err
		}

		defer resQ.Body.Close()

		resBytes, err := io.ReadAll(resQ.Body)
		if err != nil {
			return err
		}

		var jsonQueueRes map[string]any
		err = json.Unmarshal(resBytes, &jsonQueueRes)
		if err != nil {
			return err
		}

		isFound := false
		for item := range maps.Values(jsonQueueRes) {
			if len(item.([]any)) == 0 {
				continue
			}
			subitem := item.([]any)[0].([]any)

			if len(subitem) < 2 {
				continue
			}
			if subitem[1] == promptId {
				isFound = true
				break
			}
		}

		if !isFound {
			isFinished = true
		} else {
			time.Sleep(10 * time.Second)
		}

		i += 1
	}

	return nil
}

func getGenerationResultFilename(promptId string) (error, *GenerationResult) {
	log.Println("Get I2V generation result filename")

	var currentItem map[string]any

	maxAttempts := 3
	for i := 0; ; i++ {
		url := fmt.Sprintf("%s/api/history?max_items=64", os.Getenv("AI_VIDEO_HOST"))
		resH, err := http.Get(url)
		if err != nil {
			if i < maxAttempts {
				continue
			}
			return err, nil
		}

		defer resH.Body.Close()

		resBytes, err := io.ReadAll(resH.Body)
		if err != nil {
			if i < maxAttempts {
				continue
			}
			return err, nil
		}

		var jsonHistoryRes map[string]any
		err = json.Unmarshal(resBytes, &jsonHistoryRes)
		if err != nil {
			if i < maxAttempts {
				continue
			}
			return err, nil
		}

		var ok bool
		currentItem, ok = jsonHistoryRes[promptId].(map[string]any)
		if !ok {
			if i < maxAttempts {
				continue
			}
			return errors.New("Error getting generation result by key " + promptId), nil
		}

		break
	}

	outputs, ok := currentItem["outputs"].(map[string]any)
	if !ok {
		return errors.New("Error getting outputs by key " + promptId), nil
	}

	var filename string
	var subfolder string
	isFound := false
	for _, item := range outputs {
		images, ok := item.(map[string]any)["images"].([]any)
		if !ok || len(images) == 0 {
			continue
		}

		filename, ok = images[0].(map[string]any)["filename"].(string)
		if !ok {
			continue
		}
		subfolder, ok = images[0].(map[string]any)["subfolder"].(string)
		if !ok {
			continue
		}

		isFound = true
		break
	}

	if !isFound {
		return errors.New("Error getting image result by key " + promptId), nil
	}

	return nil, &GenerationResult{
		filename:  filename,
		subfolder: subfolder,
	}
}

func getGenerationResult(filename GenerationResult) (error, []byte) {
	log.Println("Start downloading I2V generation result")

	url := fmt.Sprintf("%s/api/view?filename=%s&subfolder=%s", os.Getenv("AI_VIDEO_HOST"), filename.filename, filename.subfolder)
	res, err := http.Get(url)
	if err != nil {
		return err, nil
	}

	defer res.Body.Close()

	r, err := io.ReadAll(res.Body)

	return nil, r
}
