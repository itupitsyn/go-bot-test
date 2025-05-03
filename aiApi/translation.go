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
	requestText := fmt.Sprintf(`{"model":"goekdenizguelmez/josiefied-qwen2.5-7b-abliterated-v2","messages":[{"role":"system","content":"Если эта фраза на русском, переведи её на английский. В противном случае оставь как есть. Формат вывода только результат."},{"role":"user","content":"%s"}], "stream":false}`, text)
	requestBody := []byte(requestText)

	res, err := http.Post(fmt.Sprintf("%s/api/chat", os.Getenv("AI_LLM_URL")), "application/json", bytes.NewReader(requestBody))
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

	return jsonRes["message"].(map[string]any)["content"].(string), err
}
