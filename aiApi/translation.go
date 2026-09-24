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

// chatCompletionResponse is the response of an OpenAI-compatible endpoint. It
// has many more fields, but we only need the answer text and the error message
// if the endpoint returned one instead of an answer.
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

// contentOf extracts the text of the model's answer.
//
// This used to be a chain of type assertions without a single check, and any
// answer of the wrong shape (a model refusal, empty choices, a cut-off) caused
// a panic. Handlers each run in their own goroutine, a panic in a goroutine
// takes down the whole process, so one malformed reply from the llm killed the
// bot along with everyone else's generations waiting for results at that
// moment.
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

	// An empty answer is not let through: the generator would get an empty
	// prompt and draw anything at all.
	if content == "" {
		return "", fmt.Errorf("llm returned an empty answer: %s", string(resBytes))
	}

	return content, nil
}

const translateSystemPrompt = "Если эта фраза на русском, переведи её на английский. В противном случае оставь как есть. Формат вывода только результат."

// askLLM asks the model one question and returns its answer.
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

// applyPromptTemplate substitutes the subject into a style or motion template.
//
// Trailing punctuation of the translation is cut off: the templates continue
// the sentence after their own period, and without this you get "...watching
// the sunset.. Hard-surface mecha".
func applyPromptTemplate(template, subject string) string {
	subject = strings.TrimRight(strings.TrimSpace(subject), " .,;:")

	return strings.Replace(template, "%s", subject, 1)
}
