package aiApi

import (
	"strings"
	"testing"
)

func TestGetImagePrompt(t *testing.T) {
	texts := [][]string{
		{"нарисуй котика", "котика"},
		{"нарисуй поросёнка аниме", "поросёнка"},
		{"draw meaty pork cyberpunk", "meaty pork"},
		{"Draw поросёнка аниме", "поросёнка"},
		{"DRAW a car meha", "a car"},
		{"", ""},
	}

	for _, text := range texts {

		res := getImagePrompt(text[0])
		if res != text[1] {
			t.Errorf("want %s, got %s", text[1], res)
		}
	}
}

func TestGetImageTemplate(t *testing.T) {
	texts := [][]string{
		{"нарисуй поросёнка аниме", "anime style"},
		{"draw meaty pork cyberpunk", "cyberpunk genre video games"},
		{"Draw поросёнка Anime", "anime style"},
		{"DRAW a car meha", "organic and mechanical"},
	}

	for _, text := range texts {
		res := getImageTemplate(text[0])
		if !strings.Contains(res, text[1]) {
			t.Errorf("want prompt to include %s, got %s", text[1], res)
		}
	}

}

func TestGetClearImageTemplate(t *testing.T) {
	texts := []string{"нарисуй котика", ""}

	for _, text := range texts {
		res := getImageTemplate(text)
		if res != "%s" {
			t.Errorf("want prompt to be %%s, got %s", res)
		}
	}

}
