package aiApi

// caller у генерации — кто просит (id для потолка и круга на сервисе) и куда
// сообщать о месте в очереди. Отказ по потолку приходит как ErrQueueFull.
func GetImage(msgText string, caller Caller) ([]byte, error) {
	imageData, err := generateImage(msgText, caller)
	if err != nil {
		return nil, err
	}

	return imageData, nil
}

// GetImageEdit правит присланные картинки по инструкции. Одна картинка —
// правка, несколько — микс. Промпт здесь именно инструкция («сделай волосы
// рыжими»), а не описание желаемого кадра, как в GetImage.
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

// GetSummary пересказывает текст. languageCode — тег IETF из профиля Telegram,
// может быть пустым.
func GetSummary(text, languageCode string) (string, error) {
	return generateSummary(text, languageCode)
}
