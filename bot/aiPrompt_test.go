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
		// People put a comma after the command without thinking.
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

// A punctuation mark separates the command from the prompt but does not get
// into the prompt itself.
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

// A comma also separates the command from the text of the message being replied
// to.
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

// Quotation marks in the message being replied to are a quotation and not a
// request for a spoken line: the words stay in the scene, the marks go away.
// Otherwise "я устал" would be said out loud and the scene would be left with
// "Он сказал котика".
func TestAnimationPromptLeavesTheRepliedTextAsScene(t *testing.T) {
	message := &models.Message{
		Text:           "анимируй котика",
		ReplyToMessage: &models.Message{Text: "Он сказал «я устал»"},
	}

	want := "Он сказал я устал котика"
	if got := buildAnimationPrompt(message, "анимируй", "animate"); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

// The person's own quotation marks are left alone: that is how a line is asked
// for.
func TestAnimationPromptKeepsTheTypedLine(t *testing.T) {
	message := &models.Message{
		Text:           `анимируй «мяу» медленно`,
		ReplyToMessage: &models.Message{Text: "котик на подоконнике"},
	}

	want := `котик на подоконнике «мяу» медленно`
	if got := buildAnimationPrompt(message, "анимируй", "animate"); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

// A line right after the keyword keeps its opening mark: the mark is what asks
// for the words to be spoken, and dropping it as a mere separator turned the
// request into a silent scene made of those same words.
func TestAnimationPromptKeepsTheMarkRightAfterTheKeyword(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{`Анимируй «привет, это я»`, `«привет, это я»`},
		{`Анимируй, «привет, это я»`, `«привет, это я»`},
		{`Анимируй «привет» весело`, `«привет» весело`},
		{`Анимируй "hello there"`, `"hello there"`},
	}

	for _, c := range cases {
		message := &models.Message{Text: c.text}
		if got := buildAnimationPrompt(message, "анимируй", "animate"); got != c.want {
			t.Errorf("buildAnimationPrompt(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// A mark left over in one half can no longer find its pair in the other: the
// replied text has none to pair with any more.
func TestAnimationPromptDoesNotPairMarksAcrossTheHalves(t *testing.T) {
	message := &models.Message{
		Text:           `анимируй мужика» в метро`,
		ReplyToMessage: &models.Message{Text: `он сказал «вот так`},
	}

	want := `он сказал вот так мужика» в метро`
	if got := buildAnimationPrompt(message, "анимируй", "animate"); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

// Drawing is not touched by any of this: there the marks are part of the
// picture, a sign or a label is drawn with the text as written.
func TestDrawPromptKeepsTheMarksOfTheRepliedText(t *testing.T) {
	message := &models.Message{
		Text:           "нарисуй",
		ReplyToMessage: &models.Message{Text: `вывеска «Пятёрочка»`},
	}

	want := `вывеска «Пятёрочка»`
	if got := buildAiPrompt(message, "нарисуй", "draw"); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestDropQuotationMarks(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{`он сказал «я устал»`, "он сказал я устал"},
		{`он сказал "я устал"`, "он сказал я устал"},
		{`он сказал “я устал”`, "он сказал я устал"},
		{`он сказал „я устал“`, "он сказал я устал"},
		// A mark with no spaces around it must not glue the words together.
		{`он сказал«я устал»сразу`, "он сказал я устал сразу"},
		{"котик на подоконнике", "котик на подоконнике"},
		{"", ""},
	}

	for _, c := range cases {
		if got := dropQuotationMarks(c.text); got != c.want {
			t.Errorf("dropQuotationMarks(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}
