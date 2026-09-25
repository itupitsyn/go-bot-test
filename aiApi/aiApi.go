package aiApi

// The caller of a generation says who is asking (the id the service uses for
// the per-user cap and the round-robin) and where to report the place in the
// queue. A refusal over the cap comes back as ErrQueueFull.
func GetImage(msgText string, caller Caller) ([]byte, error) {
	imageData, err := generateImage(msgText, caller)
	if err != nil {
		return nil, err
	}

	return imageData, nil
}

// GetImageEdit edits the given images following an instruction. One image means
// an edit, several mean a mix. The prompt here is an instruction ("make her
// hair red"), not a description of the desired frame as in GetImage.
func GetImageEdit(msgText string, images []EditImage, caller Caller) ([]byte, error) {
	return generateImageEdit(msgText, images, caller)
}

func GetI2V(msgText string, imageBytes []byte, imageName string, caller Caller) ([]byte, error) {
	err, video := generateI2V(msgText, imageBytes, imageName, caller)
	return video, err
}

func GetT2V(msgText string, caller Caller) ([]byte, error) {
	err, video := generateT2V(msgText, caller)
	return video, err
}

func GetTranscription(mediaBytes []byte, mediaName string, caller Caller) (string, error) {
	return generateTranscription(mediaBytes, mediaName, caller)
}

// GetSummary retells a text. languageCode is the IETF tag from the Telegram
// profile and may be empty.
func GetSummary(text, languageCode string) (string, error) {
	return generateSummary(text, languageCode)
}

// GetImageDescription describes what is in an image, in the language of the
// languageCode IETF tag from the Telegram profile (Russian when it is empty or
// unknown).
func GetImageDescription(image []byte, languageCode string) (string, error) {
	return generateImageDescription(image, languageCode)
}
