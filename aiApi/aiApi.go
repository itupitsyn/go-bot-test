package aiApi

import "time"

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

// seconds is the clip length that was asked for. Zero leaves the choice to the
// service, which means its own five seconds, and the request then goes out
// exactly as it did before lengths existed. Asking for more than the service
// allows (VIDEO_MAX_SECONDS there) is pointless: it clamps the request to what
// the model was trained on and to what it is willing to hold a card for.
func GetI2V(msgText string, imageBytes []byte, imageName string, seconds float64, caller Caller) ([]byte, error) {
	err, video := generateI2V(msgText, imageBytes, imageName, seconds, caller)
	return video, err
}

func GetT2V(msgText string, seconds float64, caller Caller) ([]byte, error) {
	err, video := generateT2V(msgText, seconds, caller)
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

// CheckFact checks a claim against a web search. A failed search comes back as
// ErrSearchUnavailable (our search server is down) or ErrSearchEnginesDown (it
// is up, but every engine behind it refused), never as "not found". The bot's
// searches share one queue; when it is over five minutes long, ErrSearchBusy.
func CheckFact(claim string) (*FactCheck, error) {
	return checkFact(claim, time.Now())
}

// GenerateFactJoke answers a message with nothing to check with a joke about
// being asked to check it. recent are the bot's last jokes in that chat, so the
// same trick does not come round again. An empty string means no joke survived.
func GenerateFactJoke(message string, recent []string) (string, error) {
	return generateFactJoke(message, recent)
}

// ReadPictureText gives the text written on a picture, word for word, for a
// fact check. An empty string means the picture has no text.
func ReadPictureText(image []byte) (string, error) {
	return readPictureText(image)
}

// GetImageDescription describes what is in an image, in the language of the
// languageCode IETF tag from the Telegram profile (Russian when it is empty or
// unknown).
func GetImageDescription(image []byte, languageCode string) (string, error) {
	return generateImageDescription(image, languageCode)
}
