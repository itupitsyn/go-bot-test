package aiApi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
)

// imageDescriptionMaxTokens caps the answer. Reasoning is switched off, so the
// whole budget goes to the description itself; with reasoning on, a small cap
// comes back as an empty answer because everything is spent on thinking.
const imageDescriptionMaxTokens = 800

// imageDescriptionSystemPrompt builds the system prompt for describing an
// image.
//
// The answer goes straight into the asker's language: describing in English
// and translating back was tried and only doubled the time. The model reads
// Cyrillic on images less reliably than Latin, but that is about reading, and
// a translation step would not fix it anyway.
//
// With an empty or unknown languageCode the answer is in Russian: unlike a
// retelling there is no source text whose language could be followed.
func imageDescriptionSystemPrompt(languageCode string) string {
	answerIn := "Отвечай на русском языке."
	if name := languageName(languageCode); name != "" {
		answerIn = fmt.Sprintf("Отвечай на %s.", name)
	}

	return "Опиши, что на картинке, коротко: два-четыре предложения, только суть. " +
		"Если на картинке есть текст, передай, что в нём сказано. " +
		"Без вступлений вроде «на картинке изображено», без списков и без заголовка. " +
		answerIn + " Формат вывода — только описание."
}

// visionRequest is a chat completion request with one image in the user turn.
type visionRequest struct {
	Messages        []visionMessage `json:"messages"`
	Stream          bool            `json:"stream"`
	ReasoningEffort string          `json:"reasoning_effort"`
	MaxTokens       int             `json:"max_tokens"`
}

type visionMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type visionPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *visionImageURL `json:"image_url,omitempty"`
}

type visionImageURL struct {
	URL string `json:"url"`
}

// buildVisionRequest assembles the request body. The image goes inline as a
// data URL: the llm host can't reach Telegram, and the server scales a large
// image down by itself, so it is sent as is.
func buildVisionRequest(image []byte, languageCode string) ([]byte, error) {
	dataURL := fmt.Sprintf("data:%s;base64,%s", http.DetectContentType(image), base64.StdEncoding.EncodeToString(image))

	return json.Marshal(visionRequest{
		Messages: []visionMessage{
			{Role: "system", Content: imageDescriptionSystemPrompt(languageCode)},
			{Role: "user", Content: []visionPart{
				{Type: "text", Text: "Что тут?"},
				{Type: "image_url", ImageURL: &visionImageURL{URL: dataURL}},
			}},
		},
		// Reasoning off is several times faster on this model, and "/no_think"
		// in the prompt no longer works on Qwen3.8.
		ReasoningEffort: "none",
		MaxTokens:       imageDescriptionMaxTokens,
	})
}

func generateImageDescription(image []byte, languageCode string) (string, error) {
	requestBody, err := buildVisionRequest(image, languageCode)
	if err != nil {
		return "", err
	}

	return postLLM(requestBody)
}
