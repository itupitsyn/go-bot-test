package bot

import (
	"strings"
	"testing"
)

func TestFormatSizePhrasePlaceholder(t *testing.T) {
	got := formatSizePhrase("Карандаш у меня {size} см", 20)
	want := "Карандаш у меня 20 см"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestFormatSizePhrasePlaceholderMultiple(t *testing.T) {
	got := formatSizePhrase("{size} см, ровно {size}", 7)
	want := "7 см, ровно 7"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestFormatSizePhraseNoPlaceholder(t *testing.T) {
	got := formatSizePhrase("У меня", 33)
	want := "У меня 33 см"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestGenerateSizeWithinBounds(t *testing.T) {
	for i := 0; i < 10000; i++ {
		got := generateSize()
		if got < sizeMin || got > sizeMax {
			t.Fatalf("generateSize() = %d, want within [%d, %d]", got, sizeMin, sizeMax)
		}
	}
}

func TestCustomSizeTemplate(t *testing.T) {
	cases := map[string]string{
		"огурец":          "Огурец у меня {size} см",
		"  баклажан  ":    "Баклажан у меня {size} см",
		"зелёный\nогурец": "Зелёный огурец у меня {size} см",
		"cucumber":        "Cucumber у меня {size} см",
		"Морковь":         "Морковь у меня {size} см",
	}
	for object, want := range cases {
		got, ok := customSizeTemplate(object)
		if !ok || got != want {
			t.Errorf("customSizeTemplate(%q) = %q, %v, want %q", object, got, ok, want)
		}
	}
	for _, empty := range []string{"", "   ", "\n"} {
		if _, ok := customSizeTemplate(empty); ok {
			t.Errorf("customSizeTemplate(%q): nothing typed must fall back to the DB phrases", empty)
		}
	}
}

func TestCustomSizeTemplateClipsLongText(t *testing.T) {
	long := strings.Repeat("огурец ", 30)
	got, _ := customSizeTemplate(long)
	object := strings.TrimSuffix(got, " у меня {size} см")
	if n := len([]rune(object)); n > maxSizeObjectRunes {
		t.Errorf("object of %d runes, want at most %d", n, maxSizeObjectRunes)
	}
	if formatSizePhrase(got, 17) != strings.ReplaceAll(got, "{size}", "17") {
		t.Error("the custom template must take the number like the DB ones")
	}
}
