package aiApi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetVideoTemplate(t *testing.T) {
	cases := [][2]string{
		{"кота медленно", "slow motion"},
		{"a cat slowly", "slow motion"},
		{"кота вокруг", "orbits the subject"},
		{"a cat around", "orbits the subject"},
		{"кота ближе", "push-in"},
		{"кота дальше", "pulls back"},
		{"кота сверху", "aerial shot"},
		{"кота живо", "energetic action"},
		{"Кота МЕДЛЕННО", "slow motion"},
	}

	for _, c := range cases {
		res := getVideoTemplate(c[0])
		if !strings.Contains(res, c[1]) {
			t.Errorf("getVideoTemplate(%q): want it to include %q, got %q", c[0], c[1], res)
		}
	}
}

func TestGetVideoTemplateFallsBackToDefault(t *testing.T) {
	cases := []string{"кота", "", "танцующего кота на столе"}

	for _, c := range cases {
		if res := getVideoTemplate(c); res != defaultVideoTemplate {
			t.Errorf("getVideoTemplate(%q): want the default template, got %q", c, res)
		}
	}
}

func TestEveryVideoTemplateMentionsSound(t *testing.T) {
	// H3 generates stereo in the same pass, so the track ends up in the mp4
	// regardless of us: every template must talk about it.
	if !strings.Contains(defaultVideoTemplate, "sound") {
		t.Errorf("the default template says nothing about sound: %q", defaultVideoTemplate)
	}

	for _, style := range videoStyles {
		lower := strings.ToLower(style.template)
		if !strings.Contains(lower, "sound") && !strings.Contains(lower, "ambience") {
			t.Errorf("template %q says nothing about sound", style.template)
		}
	}
}

func TestEveryVideoTemplateHasOnePlaceholder(t *testing.T) {
	templates := append([]string{defaultVideoTemplate}, func() []string {
		out := []string{}
		for _, style := range videoStyles {
			out = append(out, style.template)
		}
		return out
	}()...)

	for _, template := range templates {
		if strings.Count(template, "%s") != 1 {
			t.Errorf("template %q must carry exactly one %%s", template)
		}
	}
}

func TestGetVideoPrompt(t *testing.T) {
	cases := [][2]string{
		{"кота медленно", "кота"},
		{"танцующего кота вокруг", "танцующего кота"},
		{"a dancing cat slowly", "a dancing cat"},
		{"Кота СВЕРХУ", "Кота"},
		{"кота", "кота"},
		{"", ""},
		// The keyword is separated by a space, so a word that merely ends with
		// it stays whole.
		{"кота наживо", "кота наживо"},
		{"кота вокруг вокруг", "кота вокруг"},
	}

	for _, c := range cases {
		if res := getVideoPrompt(c[0]); res != c[1] {
			t.Errorf("getVideoPrompt(%q): want %q, got %q", c[0], c[1], res)
		}
	}
}

func TestGetVideoPromptLeavesNothingWhenOnlyKeyword(t *testing.T) {
	// Only emptiness is left: the caller must substitute the default subject,
	// otherwise a bare template goes to the model.
	if res := getVideoPrompt("медленно"); res != "медленно" {
		t.Errorf("одинокое слово без пробела перед ним ключевым не считается, got %q", res)
	}
}

// TestGetVideoPromptKeepsTheCaseAsTyped guards what used to be harmless. The
// subject used to be lowercased and then translated, and the llm put the case
// back; with translation off it goes to the encoder exactly as it comes out of
// here, and a proper noun in lower case is not the same input.
func TestGetVideoPromptKeepsTheCaseAsTyped(t *testing.T) {
	cases := [][2]string{
		{"Красную площадь сверху", "Красную площадь"},
		{"Чебурашку МЕДЛЕННО", "Чебурашку"},
		{"The Kremlin Closer", "The Kremlin"},
		{"Гагарина", "Гагарина"},
	}

	for _, c := range cases {
		if res := getVideoPrompt(c[0]); res != c[1] {
			t.Errorf("getVideoPrompt(%q): want %q, got %q", c[0], c[1], res)
		}
	}
}

func TestVideoTranslateIsOffUnlessAskedExplicitly(t *testing.T) {
	off := []string{"", " ", "0", "off", "no", "false", "выкл", "да", "ага"}
	for _, value := range off {
		t.Setenv(videoTranslateEnv, value)
		if translateVideoPrompts() {
			t.Errorf("%s=%q: перевод включился, а не должен", videoTranslateEnv, value)
		}
	}

	// Case and stray spaces must not decide anything: this gets typed into
	// .env.local by hand.
	on := []string{"1", "on", "yes", "true", "вкл", "  ON  ", "True"}
	for _, value := range on {
		t.Setenv(videoTranslateEnv, value)
		if !translateVideoPrompts() {
			t.Errorf("%s=%q: перевод не включился", videoTranslateEnv, value)
		}
	}
}

// TestBuildVideoPromptKeepsRussianWhenTranslationIsOff is the whole point of
// the switch: the subject reaches the model as typed, and nothing is asked of
// the llm at all. AI_LLM_URL is deliberately left pointing nowhere -- if the
// assembly still called it, the test would fail with an error instead of
// silently passing.
func TestBuildVideoPromptKeepsRussianWhenTranslationIsOff(t *testing.T) {
	t.Setenv(videoTranslateEnv, "")
	t.Setenv("AI_LLM_URL", "http://127.0.0.1:1")

	prompt, origin, err := buildVideoPrompt("Чебурашку на крыше вокруг")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, "Чебурашку на крыше") {
		t.Errorf("русский текст не доехал до промпта: %q", prompt)
	}

	// The template is still English, and that mix is on purpose.
	if !strings.Contains(prompt, "orbits the subject") {
		t.Errorf("шаблон движения потерялся: %q", prompt)
	}

	if origin.Source != "Чебурашку на крыше вокруг" || origin.Style != "orbit" {
		t.Errorf("статистика собрана неверно: %+v", origin)
	}
}

func TestBuildVideoPromptTranslatesWhenTurnedOn(t *testing.T) {
	asked := ""
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			asked = string(body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(
				`{"choices":[{"message":{"content":"a cat on a roof"}}]}`))
		}))
	defer server.Close()

	t.Setenv(videoTranslateEnv, "1")
	t.Setenv("AI_LLM_URL", server.URL)

	prompt, _, err := buildVideoPrompt("кота на крыше вокруг")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(asked, "кота на крыше") {
		t.Errorf("на перевод уехало не то: %q", asked)
	}

	if !strings.Contains(prompt, "a cat on a roof") {
		t.Errorf("перевод не попал в промпт: %q", prompt)
	}

	if strings.Contains(prompt, "кота") {
		t.Errorf("исходный текст остался рядом с переводом: %q", prompt)
	}
}

// TestBuildVideoPromptSurvivesADeadLLM pins the price of the switch being on:
// a translation failure still aborts the generation, which is one of the
// reasons it is off.
func TestBuildVideoPromptSurvivesADeadLLM(t *testing.T) {
	t.Setenv(videoTranslateEnv, "1")
	t.Setenv("AI_LLM_URL", "http://127.0.0.1:1")

	if _, _, err := buildVideoPrompt("кота"); err == nil {
		t.Error("перевод не ответил, а ошибки нет: промпт уехал бы пустым")
	}
}
