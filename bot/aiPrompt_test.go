package bot

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestIsCommand(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"нарисуй котика", true},
		{"draw a cat", true},
		{"нарисуй", true},
		{"draw", true},
		{"нарисуй\nкотика", true},
		// Запятую после команды люди ставят не задумываясь.
		{"нарисуй, котика", true},
		{"нарисуй,котика", true},
		{"нарисуй: котика", true},
		{"нарисуй!", true},
		{"draw, a cat", true},
		{"нарисуйте котика", false},
		{"drawing board", false},
		{"а нарисуй котика", false},
		{"", false},
	}

	for _, current := range cases {
		if got := isCommand(current.text, "нарисуй", "draw"); got != current.want {
			t.Errorf("isCommand(%q) = %v, want %v", current.text, got, current.want)
		}
	}
}

func TestBuildAiPromptWithoutReply(t *testing.T) {
	message := &models.Message{Text: "Нарисуй котика аниме"}

	if got := buildAiPrompt(message, "нарисуй", "draw"); got != "котика аниме" {
		t.Errorf("want %q, got %q", "котика аниме", got)
	}
}

func TestBuildAiPromptFromReply(t *testing.T) {
	message := &models.Message{
		Text:           "нарисуй",
		ReplyToMessage: &models.Message{Text: "котик на подоконнике"},
	}

	if got := buildAiPrompt(message, "нарисуй", "draw"); got != "котик на подоконнике" {
		t.Errorf("want %q, got %q", "котик на подоконнике", got)
	}
}

func TestBuildAiPromptFromReplyWithTail(t *testing.T) {
	message := &models.Message{
		Text:           "нарисуй аниме",
		ReplyToMessage: &models.Message{Text: "котик на подоконнике"},
	}

	want := "котик на подоконнике аниме"
	if got := buildAiPrompt(message, "нарисуй", "draw"); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestBuildAiPromptIgnoresNonTextReply(t *testing.T) {
	message := &models.Message{
		Text: "анимируй красиво",
		ReplyToMessage: &models.Message{
			Photo:   []models.PhotoSize{{FileID: "id"}},
			Caption: "смотрите какой закат",
		},
	}

	if got := buildAiPrompt(message, "анимируй", "animate"); got != "красиво" {
		t.Errorf("want %q, got %q", "красиво", got)
	}
}

func TestBuildAiPromptUsesCaption(t *testing.T) {
	message := &models.Message{
		Photo:   []models.PhotoSize{{FileID: "id"}},
		Caption: "Анимируй танцующим",
	}

	if got := buildAiPrompt(message, "анимируй", "animate"); got != "танцующим" {
		t.Errorf("want %q, got %q", "танцующим", got)
	}
}

// Знак препинания отделяет команду от промпта, но в сам промпт не попадает.
func TestBuildAiPromptDropsSeparator(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"Анимируй, котика", "котика"},
		{"Анимируй,котика", "котика"},
		{"Анимируй - котика", "котика"},
		{"Анимируй... котика", "котика"},
		{"Анимируй", ""},
	}

	for _, c := range cases {
		message := &models.Message{Text: c.text}
		if got := buildAiPrompt(message, "анимируй", "animate"); got != c.want {
			t.Errorf("buildAiPrompt(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// Запятая отделяет команду и от текста сообщения, на которое отвечают.
func TestBuildAiPromptWithSeparatorAndReply(t *testing.T) {
	message := &models.Message{
		Text:           "нарисуй, аниме",
		ReplyToMessage: &models.Message{Text: "котик на подоконнике"},
	}

	want := "котик на подоконнике аниме"
	if got := buildAiPrompt(message, "нарисуй", "draw"); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestIsCommandWithTwoWordKeyword(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"что тут", true},
		{"что тут написано", true},
		{"что туттакое", false},
		{"а что тут", false},
		{"чтотут", false},
		{"сократи", true},
		{"сократи это", true},
	}

	for _, c := range cases {
		if got := isCommand(c.text, "что тут", "сократи"); got != c.want {
			t.Errorf("isCommand(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestIsBareCommand(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"что тут", true},
		{"что тут?", true},
		{"что тут???", true},
		{"что тут ?", true},
		{"  что тут  ", true},
		{"что тут.", false},
		{"что тут!", false},
		{"что тут написано", false},
		{"что тут происходит?", false},
		{"а что тут", false},
		{"чтотут", false},
	}

	for _, c := range cases {
		if got := isBareCommand(c.text, "что тут"); got != c.want {
			t.Errorf("isBareCommand(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}
