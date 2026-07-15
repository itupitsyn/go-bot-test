package aiApi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func translatePrompt(text string) (string, error) {
	escaped, err := json.Marshal(text)
	if err != nil {
		return "", err
	}

	requestText := fmt.Sprintf(`{"messages":[{"role":"system","content":"Если эта фраза на русском, переведи её на английский. В противном случае оставь как есть. Формат вывода только результат."},{"role":"user","content":%s}], "stream":false, "chat_template_kwargs":{"enable_thinking":false}}`, escaped)
	requestBody := []byte(requestText)

	res, err := http.Post(fmt.Sprintf("%s/v1/chat/completions", os.Getenv("AI_LLM_URL")), "application/json", bytes.NewReader(requestBody))
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	resBody, _ := io.ReadAll(res.Body)
	resBytes := []byte(resBody)              // Converting the string "res" into byte array
	var jsonRes map[string]any               // declaring a map for key names as string and values as interface
	err = json.Unmarshal(resBytes, &jsonRes) // Unmarshalling
	if err != nil {
		return "", err
	}

	content := jsonRes["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["content"].(string)

	// Подстраховка: если модель всё же вернёт reasoning прямо в content
	// (<think>...</think>), оставляем только текст после закрывающего тега.
	// llama.cpp сейчас кладёт reasoning в отдельное поле, но бэкенд может смениться.
	if idx := strings.LastIndex(content, "</think>"); idx != -1 {
		content = content[idx+len("</think>"):]
	}
	content = strings.TrimSpace(content)

	return content, err
}
