package aiApi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// pictureTextMaxTokens leaves room for a screenshot of a whole news post.
const pictureTextMaxTokens = 1000

// pictureNoText is what the model answers for a picture without text.
const pictureNoText = "НЕТ"

// pictureTextPrompt asks for the text on a picture as it is, not for a
// description: a fact check needs the words that were claimed, and a retelling
// already changed a claim once (see factPlanPrompt).
const pictureTextPrompt = "Перепиши дословно весь текст, который есть на картинке, на языке оригинала. " +
	"Не переводи, не пересказывай, не исправляй и не добавляй описание картинки. " +
	"Пропускай элементы интерфейса: кнопки, время, счётчики лайков и просмотров, водяные знаки. " +
	"Если читаемого текста на картинке нет, ответь одним словом: " + pictureNoText + ". " +
	"Формат вывода — только текст с картинки."

func buildPictureTextRequest(image []byte) ([]byte, error) {
	dataURL := fmt.Sprintf("data:%s;base64,%s", http.DetectContentType(image), base64.StdEncoding.EncodeToString(image))

	return json.Marshal(visionRequest{
		Messages: []visionMessage{
			{Role: "system", Content: pictureTextPrompt},
			{Role: "user", Content: []visionPart{
				{Type: "text", Text: "Какой текст на картинке?"},
				{Type: "image_url", ImageURL: &visionImageURL{URL: dataURL}},
			}},
		},
		ReasoningEffort: "none",
		MaxTokens:       pictureTextMaxTokens,
	})
}

// pictureTextOf cleans the model's answer; an empty string means no text.
func pictureTextOf(answer string) string {
	answer = strings.TrimSpace(answer)
	bare := strings.ToUpper(strings.Trim(answer, " .!«»\"'`*"))
	if bare == pictureNoText || bare == pictureNoText+" ТЕКСТА" {
		return ""
	}
	return answer
}

func readPictureText(image []byte) (string, error) {
	requestBody, err := buildPictureTextRequest(image)
	if err != nil {
		return "", err
	}

	answer, err := postLLM(requestBody)
	if err != nil {
		return "", err
	}
	return pictureTextOf(answer), nil
}
