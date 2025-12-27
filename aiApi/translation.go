package aiApi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

func translatePrompt(text string) (string, error) {
	escaped, err := json.Marshal(text)
	if err != nil {
		return "", err
	}

	requestText := fmt.Sprintf(`{"messages":[{"role":"system","content":"Если эта фраза на русском, переведи её на английский. В противном случае оставь как есть. Формат вывода только результат."},{"role":"user","content":%s}], "stream":false}`, escaped)
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

	return jsonRes["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["content"].(string), err
}
