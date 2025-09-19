package aiApi

func GetImage(msgText string) ([]byte, error) {
	imageData, err := generateImage(msgText)
	if err != nil {
		return nil, err
	}

	return imageData, nil
}
