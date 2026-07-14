package aiApi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
)

type ImgSize struct {
	width  int
	height int
}

func getImageSize(imageBytes []byte) (error, *ImgSize) {
	log.Println("Calculating image size")

	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return err, nil
	}

	initialSize := ImgSize{
		width:  img.Bounds().Dx(),
		height: img.Bounds().Dy(),
	}

	ratio := float32(initialSize.width) / float32(initialSize.height)

	var width int
	var height int
	if initialSize.width > initialSize.height {
		width = 800
		height = int(float32(width) / ratio)
		height -= height % 32
	} else {
		height = 800
		width = int(float32(height) * ratio)
		width -= width % 32
	}

	return nil, &ImgSize{
		width:  width,
		height: height,
	}
}

func getI2VId(prompt string, imageBytes []byte, imageName string, imgSize ImgSize, fps int) (error, string) {
	log.Println("Start getting i2v id")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if err := writer.WriteField("prompt", prompt); err != nil {
		return err, ""
	}
	if err := writer.WriteField("width", strconv.Itoa(imgSize.width)); err != nil {
		return err, ""
	}
	if err := writer.WriteField("height", strconv.Itoa(imgSize.height)); err != nil {
		return err, ""
	}
	if err := writer.WriteField("fps", strconv.Itoa(fps)); err != nil {
		return err, ""
	}

	part, err := writer.CreateFormFile("file", imageName)
	if err != nil {
		return err, ""
	}
	if _, err = io.Copy(part, bytes.NewReader(imageBytes)); err != nil {
		return err, ""
	}
	if err = writer.Close(); err != nil {
		return err, ""
	}

	url := fmt.Sprintf("%s/api/i2v", os.Getenv("AI_VIDEO_HOST"))
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err, ""
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err, ""
	}

	defer res.Body.Close()

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return err, ""
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("i2v request failed with status %d: %s", res.StatusCode, string(resBytes)), ""
	}

	var jsonRes map[string]any
	err = json.Unmarshal(resBytes, &jsonRes)
	if err != nil {
		return fmt.Errorf("i2v response is not valid json (%w): %s", err, string(resBytes)), ""
	}

	id, ok := jsonRes["id"].(string)
	if !ok {
		return errors.New("wrong response format while getting i2v id"), ""
	}

	return nil, id
}

func generateI2V(prompt string, imageBytes []byte, imageName string) (error, []byte) {
	translatedPrompt, err := translatePrompt(prompt)
	if err != nil {
		return err, nil
	}

	err, imgSize := getImageSize(imageBytes)
	if err != nil {
		return err, nil
	}

	err, id := getI2VId(translatedPrompt, imageBytes, imageName, *imgSize, defaultVideoFps)
	if err != nil {
		return err, nil
	}

	video, err := waitVideoResult(id)
	if err != nil {
		return err, nil
	}

	return nil, video
}
