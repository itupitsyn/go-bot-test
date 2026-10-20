package aiApi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// echoLLM puts a stand-in translator in front of the assembly: it answers with
// the very text it was handed.
//
// Returning the text unchanged is what makes the rest of the tests readable.
// The scene now always goes through the llm, but these tests are about markers,
// templates and silence, not about translation -- and with an echo they can
// keep saying "мужика в метро" instead of chasing whatever English a stub
// decided to invent.
func echoLLM(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var ask struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}

			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &ask); err != nil {
				t.Errorf("перевод попросили не json-ом: %v", err)
			}

			answer := ""
			for _, message := range ask.Messages {
				if message.Role == "user" {
					answer = message.Content
				}
			}

			reply, err := json.Marshal(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]string{"content": answer}},
				},
			})
			if err != nil {
				t.Errorf("не собрался ответ подставной llm: %v", err)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(reply)
		}))

	t.Cleanup(server.Close)
	t.Setenv("AI_LLM_URL", server.URL)
}

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

func TestNeedsTranslation(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"кота на крыше", true},
		{"Чебурашку closer", true},
		{"a cat on the roof", false},
		{"the scene in the picture comes to life", false},
		{"a person faces the camera and speaks", false},
		{"", false},
	}

	for _, c := range cases {
		if got := needsTranslation(c.text); got != c.want {
			t.Errorf("needsTranslation(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

// TestEnglishSubjectIsNotSentForTranslation: there is nothing to translate in
// it, and that includes our own defaults -- the one a captionless photo comes
// with is English too. AI_LLM_URL points nowhere on purpose: an llm call would
// turn this test red instead of letting it pass quietly.
func TestEnglishSubjectIsNotSentForTranslation(t *testing.T) {
	t.Setenv("AI_LLM_URL", "http://127.0.0.1:1")

	prompt, origin, err := buildVideoPrompt("a cat on the roof around")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, "a cat on the roof") {
		t.Errorf("текст не доехал до промпта: %q", prompt)
	}
	if !strings.Contains(prompt, "orbits the subject") {
		t.Errorf("шаблон движения потерялся: %q", prompt)
	}
	if origin.Source != "a cat on the roof around" || origin.Style != "orbit" {
		t.Errorf("статистика собрана неверно: %+v", origin)
	}
}

func TestBuildVideoPromptTranslatesTheScene(t *testing.T) {
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

func TestSplitSpeechTakesTheLinesOut(t *testing.T) {
	cases := []struct {
		text  string
		scene string
		lines []string
	}{
		{`мужика в метро «я опять опоздал»`, "мужика в метро", []string{"я опять опоздал"}},
		{`мужика в метро "я опять опоздал"`, "мужика в метро", []string{"я опять опоздал"}},
		{"мужика в метро “я опять опоздал”", "мужика в метро", []string{"я опять опоздал"}},
		{"мужика в метро „я опять опоздал“", "мужика в метро", []string{"я опять опоздал"}},
		// The line may stand anywhere, and the words around it stay the scene.
		{`кот «мяу» на диване`, "кот на диване", []string{"мяу"}},
		// Two lines are two lines, in the order written.
		{`спор «ты кто» «я сам кто»`, "спор", []string{"ты кто", "я сам кто"}},
		// Nothing quoted at all: everything is the scene.
		{"танцующего кота", "танцующего кота", []string{}},
		// Empty quotes are not a line.
		{`кота «»`, "кота", []string{}},
	}

	for _, c := range cases {
		scene, lines := splitSpeech(c.text)
		if scene != c.scene {
			t.Errorf("splitSpeech(%q): сцена %q, ждали %q", c.text, scene, c.scene)
		}
		if strings.Join(lines, "|") != strings.Join(c.lines, "|") {
			t.Errorf("splitSpeech(%q): реплики %q, ждали %q", c.text, lines, c.lines)
		}
	}
}

// TestSplitSpeechLeavesAnUnpairedQuoteAlone: a forgotten closing quote must not
// turn the rest of the request into a line. Otherwise a typo produces a video
// of someone reading the request out loud -- the very thing being fixed here.
func TestSplitSpeechLeavesAnUnpairedQuoteAlone(t *testing.T) {
	scene, lines := splitSpeech(`кота «на подоконнике`)

	if len(lines) != 0 {
		t.Errorf("непарная кавычка стала репликой: %q", lines)
	}
	if scene != `кота «на подоконнике` {
		t.Errorf("сцена пострадала: %q", scene)
	}
}

// TestSplitSpeechStripsModelMarkers: the dialogue markers are ours, not the
// person's. Typed by hand they must not reach the model as control tokens.
func TestSplitSpeechStripsModelMarkers(t *testing.T) {
	scene, lines := splitSpeech(`кота <d>сам себе</d> «мяу» <|cutoff|>`)

	if strings.Contains(scene, "<d>") || strings.Contains(scene, "cutoff") {
		t.Errorf("метки модели просочились из текста человека: %q", scene)
	}
	if scene != "кота сам себе" {
		t.Errorf("сцена собрана неверно: %q", scene)
	}
	if len(lines) != 1 || lines[0] != "мяу" {
		t.Errorf("реплика потерялась: %q", lines)
	}
}

// TestKeywordIsStillFoundAfterALineIsCutOut guards the order of operations: the
// line is lifted out first, so the camera keyword ends up at the end of what
// remains, which is the only place getVideoTemplate looks.
func TestKeywordIsStillFoundAfterALineIsCutOut(t *testing.T) {
	echoLLM(t)

	prompt, origin, err := buildVideoPrompt(`мужика в метро «я опоздал» медленно`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if origin.Style != "slow"+speechStyleMark {
		t.Errorf("стиль определён неверно: %q", origin.Style)
	}
	if !strings.Contains(prompt, "slow motion") {
		t.Errorf("шаблон движения не применился: %q", prompt)
	}
	if !strings.Contains(prompt, speechOpen+"я опоздал"+speechClose) {
		t.Errorf("реплика не размечена: %q", prompt)
	}
}

func TestSceneRequestAsksForSilence(t *testing.T) {
	echoLLM(t)

	prompt, origin, err := buildVideoPrompt("танцующего кота")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(prompt, speechOpen) {
		t.Errorf("в сцене без кавычек появилась разметка реплики: %q", prompt)
	}
	if !strings.Contains(prompt, noSpeechClause) {
		t.Errorf("сцена не просит тишины, а это и есть лечение зачитывания: %q", prompt)
	}
	if origin.Style != defaultVideoStyleName {
		t.Errorf("сцене приписали речь: %q", origin.Style)
	}
}

func TestSpeechRequestDoesNotAskForSilence(t *testing.T) {
	echoLLM(t)

	prompt, _, err := buildVideoPrompt(`мужика в метро «я опоздал»`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(prompt, noSpeechClause) {
		t.Errorf("просим и говорить, и молчать одновременно: %q", prompt)
	}
	if !strings.Contains(prompt, "мужика в метро") {
		t.Errorf("сцена не доехала: %q", prompt)
	}
}

// TestLineOnlyRequestGetsASpeaker: a request that is nothing but a line still
// needs someone in frame, otherwise the template wraps emptiness.
func TestLineOnlyRequestGetsASpeaker(t *testing.T) {
	echoLLM(t)

	prompt, _, err := buildVideoPrompt(`«привет, это я»`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, defaultSpeakerSubject) {
		t.Errorf("говорить некому: %q", prompt)
	}
	if !strings.Contains(prompt, speechOpen+"привет, это я"+speechClose) {
		t.Errorf("реплика не размечена: %q", prompt)
	}
}

// TestSpokenLineIsNeverTranslated is the one that would silently ruin the
// feature: the scene goes to the llm, but the line must reach the model in the
// words it was written in -- it is what the viewer will hear.
func TestSpokenLineIsNeverTranslated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "я опоздал") {
				t.Error("реплика уехала на перевод")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(
				`{"choices":[{"message":{"content":"a man in the metro"}}]}`))
		}))
	defer server.Close()

	t.Setenv("AI_LLM_URL", server.URL)

	prompt, _, err := buildVideoPrompt(`мужика в метро «я опоздал»`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, speechOpen+"я опоздал"+speechClose) {
		t.Errorf("реплика не дошла по-русски: %q", prompt)
	}
	if !strings.Contains(prompt, "a man in the metro") {
		t.Errorf("сцена не переведена: %q", prompt)
	}
}

// TestDeadLLMStopsTheVideo pins the price we agreed to pay for translating
// every scene: when the llm does not answer, the generation stops with an
// error. That is the lesser evil -- the alternative is three minutes of card
// time spent on a prompt the model reads wrong, which is what sent "человек
// застрелил трусы" off into a beauty vlog.
func TestDeadLLMStopsTheVideo(t *testing.T) {
	t.Setenv("AI_LLM_URL", "http://127.0.0.1:1")

	if _, _, err := buildVideoPrompt("кота"); err == nil {
		t.Error("перевод не ответил, а ошибки нет: промпт уехал бы пустым")
	}
}
