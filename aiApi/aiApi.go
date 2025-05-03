package aiApi

import (
	"github.com/google/uuid"
)

func GetImage(msgText string) (string, error) {
	hash := string(uuid.NewString()[:8])
	_, err := initiateImageGeneration(hash, msgText)
	if err != nil {
		return "", err
	}
	url, err := processGenerationResult(hash)
	if err != nil {
		return "", err
	}
	return url, nil
}
