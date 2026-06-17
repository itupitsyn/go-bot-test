package bot

import "testing"

func TestFormatDepthPhrasePlaceholder(t *testing.T) {
	got := formatDepthPhrase("Глубина Матильды {depth} см", 20)
	want := "Глубина Матильды 20 см"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestFormatDepthPhrasePlaceholderMultiple(t *testing.T) {
	got := formatDepthPhrase("{depth} м, ровно {depth}", 7)
	want := "7 м, ровно 7"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestFormatDepthPhraseNoPlaceholder(t *testing.T) {
	got := formatDepthPhrase("Глубина", 33)
	want := "Глубина 33 см"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestGenerateDepthWithinBounds(t *testing.T) {
	for i := 0; i < 10000; i++ {
		got := generateDepth()
		if got < depthMin || got > depthMax {
			t.Fatalf("generateDepth() = %d, want within [%d, %d]", got, depthMin, depthMax)
		}
	}
}
