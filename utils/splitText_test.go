package utils

import (
	"strings"
	"testing"
)

func TestSplitTextShortStaysWhole(t *testing.T) {
	cases := []string{"", "кот", strings.Repeat("a", 10)}

	for _, c := range cases {
		res := SplitText(c, 10)
		if len(res) != 1 || res[0] != c {
			t.Errorf("SplitText(%q, 10): want [%q], got %q", c, c, res)
		}
	}
}

func TestSplitTextBreaksOnSpace(t *testing.T) {
	res := SplitText("кот сидит на окне", 10)

	for _, chunk := range res {
		if len([]rune(chunk)) > 10 {
			t.Errorf("chunk %q is longer than the limit of 10 runes", chunk)
		}
		if strings.TrimSpace(chunk) != chunk {
			t.Errorf("chunk %q is not trimmed", chunk)
		}
	}

	// Every word fits the limit, so nothing gets cut in half and the chunks
	// join back into the original.
	if joined := strings.Join(res, " "); joined != "кот сидит на окне" {
		t.Errorf("joining the chunks back: got %q", joined)
	}
}

func TestSplitTextPrefersLineBreak(t *testing.T) {
	res := SplitText("первая строка\nвторая строка", 20)

	if len(res) != 2 || res[0] != "первая строка" || res[1] != "вторая строка" {
		t.Errorf("want the two lines split apart, got %q", res)
	}
}

func TestSplitTextCutsWordWithoutSeparator(t *testing.T) {
	// A single unbroken word has no space to break on, so it gets cut at the
	// limit rather than overflowing it.
	res := SplitText(strings.Repeat("ы", 25), 10)

	if len(res) != 3 {
		t.Fatalf("want 3 chunks, got %d: %q", len(res), res)
	}
	for _, chunk := range res[:2] {
		if len([]rune(chunk)) != 10 {
			t.Errorf("want a full chunk of 10 runes, got %d", len([]rune(chunk)))
		}
	}
}

func TestSplitTextCountsRunesNotBytes(t *testing.T) {
	// Cyrillic is two bytes per rune: cutting by bytes would both split a rune
	// in half and produce far more chunks than the limit calls for.
	res := SplitText(strings.Repeat("я", 100), 100)

	if len(res) != 1 {
		t.Errorf("100 runes at a limit of 100 should stay whole, got %d chunks", len(res))
	}
}
