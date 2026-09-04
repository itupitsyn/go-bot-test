package aiApi

// onProgress у генерации получает место задачи в очереди сервиса на каждом
// опросе результата; nil — не отслеживать очередь.
func GetImage(msgText string, onProgress ProgressFunc) ([]byte, error) {
	imageData, err := generateImage(msgText, onProgress)
	if err != nil {
		return nil, err
	}

	return imageData, nil
}

func GetI2V(msgText string, imageBytes []byte, imageName string, onProgress ProgressFunc) ([]byte, error) {
	err, video := generateI2V(msgText, imageBytes, imageName, onProgress)
	return video, err
}

func GetT2V(msgText string, onProgress ProgressFunc) ([]byte, error) {
	err, video := generateT2V(msgText, onProgress)
	return video, err
}

func GetTranscription(mediaBytes []byte, mediaName string, onProgress ProgressFunc) (string, error) {
	return generateTranscription(mediaBytes, mediaName, onProgress)
}

// GetSummary пересказывает текст. languageCode — тег IETF из профиля Telegram,
// может быть пустым.
func GetSummary(text, languageCode string) (string, error) {
	return generateSummary(text, languageCode)
}
