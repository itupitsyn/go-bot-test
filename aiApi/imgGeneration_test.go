package aiApi

import (
	"strings"
	"testing"
)

func TestGetImagePrompt(t *testing.T) {
	texts := [][]string{
		{"нарисуй котика", "котика"},
		{"Draw поросёнка", "поросёнка"},
		{"DRAW a car", "a car"},
		{"  нарисуй   кота  ", "кота"},
		// The command word is cut only at the front: a picture may well be of
		// someone drawing.
		{"мальчик рисует draw", "мальчик рисует draw"},
		{"", ""},
	}

	for _, text := range texts {
		res := getImagePrompt(text[0])
		if res != text[1] {
			t.Errorf("want %q, got %q", text[1], res)
		}
	}
}

// Case is no longer flattened: the picture model draws the letters it is
// given, and a sign asked for in capitals has to arrive in capitals. This is
// the regression that the old lowercasing caused.
func TestGetImagePromptKeepsCase(t *testing.T) {
	texts := [][]string{
		{"нарисуй вывеску «КОФЕ С СОБОЙ»", "вывеску «КОФЕ С СОБОЙ»"},
		{"Lada Vesta with Khan body kit", "Lada Vesta with Khan body kit"},
	}

	for _, text := range texts {
		res := getImagePrompt(text[0])
		if res != text[1] {
			t.Errorf("want %q, got %q", text[1], res)
		}
	}
}

// restoreQuoted is the guard that makes the enhancement safe to ship: the
// model normalises «ёлочки» into "straight quotes" no matter how plainly the
// instruction forbids it, and the picture model then draws the wrong ones.
func TestRestoreQuotedPutsMarksBack(t *testing.T) {
	source := "открытка с надписью «С днём рождения, Марина!»"
	enhanced := `A watercolour greeting card with the text "С днём рождения, Марина!" in the centre.`

	got, ok := restoreQuoted(enhanced, source)
	if !ok {
		t.Fatalf("quoted text was present, want ok")
	}

	if want := "«С днём рождения, Марина!»"; !strings.Contains(got, want) {
		t.Errorf("want %q restored, got %q", want, got)
	}
}

func TestRestoreQuotedKeepsCorrectMarks(t *testing.T) {
	source := "вывеска «КОФЕ»"
	enhanced := "A shop sign reading «КОФЕ» above the door, evening light."

	got, ok := restoreQuoted(enhanced, source)
	if !ok || got != enhanced {
		t.Errorf("nothing to repair, want the text untouched, got %q (ok=%v)", got, ok)
	}
}

// A rewrite that dropped the person's words is worse than no rewrite at all:
// they asked for those words to be in the picture.
func TestRestoreQuotedRejectsLostText(t *testing.T) {
	source := "вывеска «КОФЕ»"
	enhanced := "A cosy coffee shop at dusk with a glowing sign above the door."

	if _, ok := restoreQuoted(enhanced, source); ok {
		t.Errorf("quoted text vanished, want it rejected")
	}
}

func TestAcceptEnhancedBounds(t *testing.T) {
	long := ""
	for range enhanceMaxWords + 5 {
		long += "word "
	}

	cases := []struct {
		name string
		text string
		want bool
	}{
		{"empty", "   ", false},
		{"too short", "a cat", false},
		{"a wall of text", long, false},
		{
			"ordinary answer",
			"A ginger cat sits on a sunlit windowsill in vivid anime style, " +
				"soft afternoon light through the glass, dust in the air.",
			true,
		},
	}

	for _, c := range cases {
		if _, ok := acceptEnhanced(c.text, "кот"); ok != c.want {
			t.Errorf("%s: want ok=%v, got %v", c.name, c.want, ok)
		}
	}
}

// acceptEnhanced also squeezes whitespace: the model sometimes answers with
// line breaks, and the prompt travels as one line.
func TestAcceptEnhancedCollapsesWhitespace(t *testing.T) {
	text := "A ginger cat on a windowsill,\n\n  anime style, soft light, dust in the air, quiet afternoon mood."

	got, ok := acceptEnhanced(text, "кот")
	if !ok {
		t.Fatalf("want the answer accepted")
	}

	if strings.Contains(got, "\n") || strings.Contains(got, "  ") {
		t.Errorf("want whitespace squeezed, got %q", got)
	}
}
