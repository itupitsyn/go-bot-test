package aiApi

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

func testPNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func TestBuildVisionRequest(t *testing.T) {
	body, err := buildVisionRequest(testPNG(t), "de")
	if err != nil {
		t.Fatal(err)
	}

	var req struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		ReasoningEffort string `json:"reasoning_effort"`
		MaxTokens       int    `json:"max_tokens"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("тело не json: %v", err)
	}

	// With reasoning on and a capped answer the model returns an empty string.
	if req.ReasoningEffort != "none" {
		t.Errorf("reasoning_effort = %q, want none", req.ReasoningEffort)
	}
	if req.MaxTokens <= 0 {
		t.Errorf("max_tokens = %d", req.MaxTokens)
	}

	if len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
		t.Fatalf("ожидались system и user, пришло %s", body)
	}
	if !strings.Contains(string(req.Messages[0].Content), "немецком языке") {
		t.Errorf("системный промпт не называет язык: %s", req.Messages[0].Content)
	}

	var parts []struct {
		Type     string `json:"type"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(req.Messages[1].Content, &parts); err != nil {
		t.Fatalf("user content не список частей: %v", err)
	}

	var url string
	for _, p := range parts {
		if p.Type == "image_url" && p.ImageURL != nil {
			url = p.ImageURL.URL
		}
	}
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Errorf("картинка ушла не data URL с верным типом: %.40s", url)
	}
}

func TestImageDescriptionSystemPromptFallsBackToRussian(t *testing.T) {
	// Unlike a retelling there is no source text to take the language from.
	for _, code := range []string{"", "xx"} {
		if prompt := imageDescriptionSystemPrompt(code); !strings.Contains(prompt, "русском языке") {
			t.Errorf("imageDescriptionSystemPrompt(%q): %s", code, prompt)
		}
	}
}

// A probe: it sends a real image to the live llm and prints the descriptions
// in several languages. It asserts nothing; read by eye.
//
//	AI_VISION_PROBE=path/to/image.jpg go test ./aiApi/ -run TestImageDescriptionProbe -v -timeout 10m
func TestImageDescriptionProbe(t *testing.T) {
	path := os.Getenv("AI_VISION_PROBE")
	if path == "" {
		t.Skip("пробник выключен; запуск: AI_VISION_PROBE=path/to/image.jpg go test ./aiApi/ -run TestImageDescriptionProbe -v -timeout 10m")
	}
	if os.Getenv("AI_LLM_URL") == "" {
		_ = godotenv.Load("../.env.local")
	}
	if os.Getenv("AI_LLM_URL") == "" {
		t.Skip("AI_LLM_URL не задан")
	}

	image, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	for _, code := range []string{"ru", "en", ""} {
		description, err := generateImageDescription(image, code)
		if err != nil {
			t.Errorf("код %q: %v", code, err)
			continue
		}
		t.Logf("[%s] %s", code, description)
	}
}
