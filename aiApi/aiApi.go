package aiApi

func GetImage(msgText string) ([]byte, error) {
	imageData, err := generateImage(msgText)
	if err != nil {
		return nil, err
	}

	return imageData, nil
}

func GetI2V(msgText string, imageBytes []byte, imageName string) ([]byte, error) {
	err, video := generateI2V(msgText, imageBytes, imageName)
	return video, err
}
