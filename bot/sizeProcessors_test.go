package bot

import "testing"

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
