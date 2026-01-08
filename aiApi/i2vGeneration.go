package aiApi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder	"io"
	"io"
	"log"
	"math"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
)

type ImgSize struct {
	width  int
	height int
}

func getI2VpromptId(prompt string, imageName string, imgSize ImgSize) (error, string) {
	log.Println("Start getting prompt id")

	escaped, err := json.Marshal(prompt)
	if err != nil {
		return err, ""
	}

	jsonStr := strings.ReplaceAll(i2vPrompt, `"PositivePrompt"`, string(escaped))
	jsonStr = strings.ReplaceAll(jsonStr, "206275406212235", fmt.Sprint(rand.Int64N(math.MaxInt64)))
	jsonStr = strings.ReplaceAll(jsonStr, "ImageName", imageName)
	jsonStr = strings.ReplaceAll(jsonStr, "800", fmt.Sprint(imgSize.width))
	jsonStr = strings.ReplaceAll(jsonStr, "496", fmt.Sprint(imgSize.height))
	url := fmt.Sprintf("%s/api/prompt", os.Getenv("AI_VIDEO_HOST"))

	resP, err := http.Post(url, "application/json", bytes.NewReader([]byte(jsonStr)))
	if err != nil {
		return err, ""
	}

	defer resP.Body.Close()

	resBytes, err := io.ReadAll(resP.Body)
	if err != nil {
		return err, ""
	}

	var promptJsonRes map[string]any
	err = json.Unmarshal(resBytes, &promptJsonRes)
	if err != nil {
		return err, ""
	}

	promptId, ok := promptJsonRes["prompt_id"].(string)
	if !ok {
		return errors.New("Error getting prompt_id"), ""
	}
	return nil, promptId
}

func uploadImage(imageBytes []byte, filename string) (error, string) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 2. Add a text field
	_ = writer.WriteField("type", "input")

	// 3. Add a file
	part, err := writer.CreateFormFile("image", filename)
	if err != nil {
		return err, ""
	}
	// Copy the byte array data into the form file field writer
	_, err = io.Copy(part, bytes.NewReader(imageBytes))
	if err != nil {
		return err, ""
	}
	// 4. Important: close the writer to finalize the body
	writer.Close()

	// 5. Create the HTTP request
	url := fmt.Sprintf("%s/api/upload/image", os.Getenv("AI_VIDEO_HOST"))
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err, ""
	}

	// 6. Important: set the Content-Type header with the boundary
	// The writer.Boundary() method provides the correct header value.
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 7. Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
	}
	defer resp.Body.Close()

	resBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err, ""
	}

	var jsonResp map[string]any
	err = json.Unmarshal(resBytes, &jsonResp)
	if err != nil {
		return err, ""
	}

	name, ok := jsonResp["name"].(string)

	if !ok {
		return errors.New("Error getting uploaded filename"), ""
	}

	return nil, name
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

func generateI2V(prompt string, imageBytes []byte, imageName string) (error, []byte) {
	translatedPrompt, err := translatePrompt(prompt)

	if err != nil {
		return err, nil
	}

	err, imgSize := getImageSize(imageBytes)
	if err != nil {
		return err, nil
	}

	err, uploadedImageName := uploadImage(imageBytes, imageName)
	if err != nil {
		return err, nil
	}

	err, promptId := getI2VpromptId(translatedPrompt, uploadedImageName, *imgSize)
	if err != nil {
		return err, nil
	}

	err = waitUntilGenerationFinished(promptId)
	if err != nil {
		return err, nil
	}

	err, filename := getGenerationResultFilename(promptId)
	if err != nil {
		return err, nil
	}

	err, video := getGenerationResult(*filename)

	return nil, video
}
