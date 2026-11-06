package aiApi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPictureTextOf(t *testing.T) {
	for answer, want := range map[string]string{
		"НЕТ":        "",
		" нет. ":     "",
		"«НЕТ»":      "",
		"Нет текста": "",
		"Сбербанк повысил ставки": "Сбербанк повысил ставки",
		"  Нет, это не так  \n":   "Нет, это не так",
		"":                        "",
	} {
		if got := pictureTextOf(answer); got != want {
			t.Errorf("pictureTextOf(%q) = %q, want %q", answer, got, want)
		}
	}
}

func TestPictureTextRequest(t *testing.T) {
	body, err := buildPictureTextRequest([]byte("\x89PNG\r\n\x1a\nxxxx"))
	if err != nil {
		t.Fatal(err)
	}
	var request visionRequest
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	if request.ReasoningEffort != "none" || request.MaxTokens != pictureTextMaxTokens {
		t.Fatalf("request settings: %+v", request)
	}
	if !strings.Contains(string(body), "data:image/png;base64,") {
		t.Fatal("image is not sent inline")
	}
	if !strings.Contains(string(body), "дословно") {
		t.Fatal("system prompt is not the text-reading one")
	}
}
