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

// chatCompletionResponse — ответ OpenAI-совместимой ручки. Полей в нём куда
// больше, но нам нужен только текст ответа и сообщение об ошибке, если ручка
// вернула её вместо ответа.
type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// contentOf достаёт текст ответа модели.
//
// Раньше это была цепочка приведений типа без единой проверки, и любой ответ
// не той формы — отказ модели, пустой choices, обрыв — ронял панику. Хендлеры
// крутятся каждый в своей горутине, паника в горутине кладёт процесс целиком,
// так что одна кривая отдача от llm убивала бота вместе со всеми чужими
// генерациями, которые в тот момент ждали результата.
func contentOf(resBytes []byte) (string, error) {
	var parsed chatCompletionResponse
	if err := json.Unmarshal(resBytes, &parsed); err != nil {
		return "", fmt.Errorf("llm response is not valid json (%w): %s", err, string(resBytes))
	}

	if parsed.Error != nil {
		return "", fmt.Errorf("llm returned an error: %s", parsed.Error.Message)
	}

	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices: %s", string(resBytes))
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)

	// Пустой ответ дальше не пускаем: генератор получил бы пустой промпт и
	// нарисовал бы что угодно.
	if content == "" {
		return "", fmt.Errorf("llm returned an empty answer: %s", string(resBytes))
	}

	return content, nil
}

const translateSystemPrompt = "Если эта фраза на русском, переведи её на английский. В противном случае оставь как есть. Формат вывода только результат."

// askLLM задаёт модели один вопрос и возвращает её ответ.
func askLLM(systemPrompt, userText string) (string, error) {
	escapedSystem, err := json.Marshal(systemPrompt)
	if err != nil {
		return "", err
	}

	escapedUser, err := json.Marshal(userText)
	if err != nil {
		return "", err
	}

	requestText := fmt.Sprintf(`{"messages":[{"role":"system","content":%s},{"role":"user","content":%s}], "stream":false, "chat_template_kwargs":{"enable_thinking":false}}`, escapedSystem, escapedUser)
	requestBody := []byte(requestText)

	res, err := llmClient.Post(fmt.Sprintf("%s/v1/chat/completions", os.Getenv("AI_LLM_URL")), "application/json", bytes.NewReader(requestBody))
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm request failed with status %d: %s", res.StatusCode, string(resBytes))
	}

	return contentOf(resBytes)
}

func translatePrompt(text string) (string, error) {
	return askLLM(translateSystemPrompt, text)
}

// applyPromptTemplate подставляет субъект в шаблон стиля или движения.
//
// Хвостовая пунктуация перевода срезается: шаблоны продолжают фразу со своей
// точки, и без этого выходит «...watching the sunset.. Hard-surface mecha».
func applyPromptTemplate(template, subject string) string {
	subject = strings.TrimRight(strings.TrimSpace(subject), " .,;:")

	return strings.Replace(template, "%s", subject, 1)
}
