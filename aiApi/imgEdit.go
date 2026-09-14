package aiApi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
)

// EditImage — картинка, уезжающая на правку: байты и имя файла для multipart.
type EditImage struct {
	Bytes []byte
	Name  string
}

// editMaxImages — столько же, сколько принимает сервис. Держим проверку и
// здесь, чтобы не гонять мегабайты ради заведомого 400.
const editMaxImages = 3

// getEditInstruction готовит текст для /api/edit.
//
// В отличие от txt2img тут НЕ надо срезать хвосты стилей: «сделай её аниме» —
// это законная инструкция, а не пожелание к рендеру. Срезаем только само
// ключевое слово команды.
//
// Переводим на английский по той же причине, что и промпты генерации: у
// Qwen2.5-VL, который читает инструкцию и картинку, русский заметно слабее
// английского.
func getEditInstruction(msgText string) (string, error) {
	text := strings.TrimSpace(msgText)
	for _, keyword := range []string{"нарисуй ", "draw "} {
		if cut, ok := strings.CutPrefix(strings.ToLower(text), keyword); ok {
			text = strings.TrimSpace(cut)
			break
		}
	}

	if text == "" {
		return "", errors.New("empty edit instruction")
	}

	return translatePrompt(text)
}

func getEditId(prompt string, images []EditImage, caller Caller) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if err := writer.WriteField("prompt", prompt); err != nil {
		return "", err
	}
	if user := caller.userForm(); user != "" {
		if err := writer.WriteField("user", user); err != nil {
			return "", err
		}
	}

	// Поле называется files и повторяется: сервис принимает список и трактует
	// несколько картинок как микс — «возьми женщину со второй и посади за стол
	// с первой».
	for _, img := range images {
		part, err := writer.CreateFormFile("files", img.Name)
		if err != nil {
			return "", err
		}
		if _, err = io.Copy(part, bytes.NewReader(img.Bytes)); err != nil {
			return "", err
		}
	}

	if err := writer.Close(); err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/api/edit", os.Getenv("AI_PAINTER_HOST"))
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	res, err := submitClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	if err := checkSubmitStatus("edit", res.StatusCode, resBytes); err != nil {
		return "", err
	}

	var jsonRes map[string]any
	if err := json.Unmarshal(resBytes, &jsonRes); err != nil {
		return "", fmt.Errorf("edit response is not valid json (%w): %s", err, string(resBytes))
	}

	id, ok := jsonRes["id"].(string)
	if !ok {
		return "", errors.New("wrong response format while getting edit id")
	}

	return id, nil
}

func generateImageEdit(msgText string, images []EditImage, caller Caller) ([]byte, error) {
	if len(images) == 0 {
		return nil, errors.New("no images to edit")
	}
	if len(images) > editMaxImages {
		images = images[:editMaxImages]
	}

	instruction, err := getEditInstruction(msgText)
	if err != nil {
		return nil, err
	}
	log.Printf("Edit instruction: %s (%d image(s))\n", instruction, len(images))

	id, err := getEditId(instruction, images, caller)
	if err != nil {
		return nil, err
	}

	return waitResult("edit", os.Getenv("AI_PAINTER_HOST"), id, imagePollInterval, imageMaxWait, caller.Progress)
}
