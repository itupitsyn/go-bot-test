package aiApi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// planLLM stands in for the llm: it answers every request with the given plan
// and remembers what it was asked.
func planLLM(t *testing.T, answer string) *string {
	t.Helper()

	asked := new(string)
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			*asked = string(body)

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

	return asked
}

func TestEveryVideoTemplateMentionsSound(t *testing.T) {
	templates := map[string]string{defaultVideoStyleName: defaultVideoTemplate}
	for name, template := range videoTemplates {
		templates[name] = template
	}

	for name, template := range templates {
		lowered := strings.ToLower(template)
		if !strings.Contains(lowered, "sound") && !strings.Contains(lowered, "ambience") {
			t.Errorf("шаблон %q молчит про звук: H3 всё равно его сгенерирует", name)
		}
	}
}

func TestEveryVideoTemplateHasOnePlaceholder(t *testing.T) {
	templates := map[string]string{defaultVideoStyleName: defaultVideoTemplate}
	for name, template := range videoTemplates {
		templates[name] = template
	}

	for name, template := range templates {
		if strings.Count(template, "%s") != 1 {
			t.Errorf("шаблон %q должен содержать ровно одно место для сцены", name)
		}
	}
}

// TestBuildVideoPromptFollowsThePlan is the whole contract in one test: the
// scene comes back in English, the line in the language it was typed in, and
// the camera chooses the template.
func TestBuildVideoPromptFollowsThePlan(t *testing.T) {
	asked := planLLM(t, `{"scene":"A man is in a subway station and then he says:",
		"lines":["я опоздал"],"camera":"slow"}`)

	prompt, origin, err := buildVideoPrompt(`мужика в метро «я опоздал» медленно`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(*asked, "мужика в метро") {
		t.Errorf("на разбор уехало не то: %q", *asked)
	}

	want := "A man is in a subway station and then he says: " +
		speechOpen + "я опоздал" + speechClose +
		". The action unfolds in slow motion"
	if !strings.Contains(prompt, want) {
		t.Errorf("промпт собран неверно: %q", prompt)
	}

	if strings.Contains(prompt, noSpeechClause) {
		t.Errorf("просим и говорить, и молчать одновременно: %q", prompt)
	}

	if origin.Source != `мужика в метро «я опоздал» медленно` {
		t.Errorf("исходный запрос потерялся: %+v", origin)
	}
	if origin.Style != "slow"+speechStyleMark {
		t.Errorf("стиль собран неверно: %q", origin.Style)
	}
}

// TestSpokenLineSitsNextToTheScene pins the order inside the prompt: the line
// stands where the words introducing it leave off, ahead of the camera and
// sound wording. Put at the end instead, it made the model read the template
// out loud (20.10.2026).
func TestSpokenLineSitsNextToTheScene(t *testing.T) {
	planLLM(t, `{"scene":"A man shoots a pair of underwear and then says:",
		"lines":["Я покакал"],"camera":"none"}`)

	prompt, _, err := buildVideoPrompt(
		`человек стреляет в трусы, а потом говорит: "Я покакал"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	line := strings.Index(prompt, speechOpen+"Я покакал"+speechClose)
	if line < 0 {
		t.Fatalf("реплика не размечена: %q", prompt)
	}
	if !strings.Contains(prompt[:line], "and then says:") {
		t.Errorf("сцена потеряла слова, которые вводят реплику: %q", prompt)
	}
	if template := strings.Index(prompt, "Natural smooth motion"); template < line {
		t.Errorf("шаблон движения стоит там, где модель ждёт реплику: %q", prompt)
	}
}

func TestSceneRequestAsksForSilence(t *testing.T) {
	planLLM(t, `{"scene":"A cat is dancing.","lines":[],"camera":"none"}`)

	prompt, origin, err := buildVideoPrompt("танцующего кота")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(prompt, speechOpen) {
		t.Errorf("в молчаливой сцене появилась разметка реплики: %q", prompt)
	}
	if !strings.Contains(prompt, noSpeechClause) {
		t.Errorf("сцена не просит тишины, а это и есть лечение зачитывания: %q", prompt)
	}
	if origin.Style != defaultVideoStyleName {
		t.Errorf("сцене приписали речь: %q", origin.Style)
	}
}

// TestLineOnlyRequestGetsASpeaker: the llm returns an empty scene for a request
// that is nothing but a line, and the template would wrap emptiness.
func TestLineOnlyRequestGetsASpeaker(t *testing.T) {
	planLLM(t, `{"scene":"","lines":["привет, это я"],"camera":"none"}`)

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

// A line that came back changed is used anyway: the request must not die over
// it.
//
// Until 21.10.2026 this very case was an error -- the line had to appear in the
// request verbatim. It cost a live video: the llm turned «ребятки» into
// «ребята», the check refused the plan, and the person got "сервер подох"
// instead of a clip. The only thing asking for the words to be exact is now
// videoPlanPrompt, and this test exists to keep the refusal from coming back by
// accident.
func TestRewordedLineIsUsedAnyway(t *testing.T) {
	planLLM(t, `{"scene":"A man is speaking and then he says:",
		"lines":["привет, как дела"],"camera":"none"}`)

	prompt, origin, err := buildVideoPrompt(`мужик говорит: "превет, как делаа"`)
	if err != nil {
		t.Fatalf("исправленная реплика завалила запрос: %v", err)
	}

	if !strings.Contains(prompt, speechOpen+"привет, как дела"+speechClose) {
		t.Errorf("реплика не доехала до промпта: %q", prompt)
	}
	if !strings.HasSuffix(origin.Style, speechStyleMark) {
		t.Errorf("речь не отмечена в статистике: %q", origin.Style)
	}
}

// A camera we have no template for is not worth failing over: the video simply
// comes out without the motion.
func TestUnknownCameraFallsBackToTheDefault(t *testing.T) {
	planLLM(t, `{"scene":"A cat is dancing.","lines":[],"camera":"dolly-zoom"}`)

	prompt, origin, err := buildVideoPrompt("танцующего кота")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, "Natural smooth motion") {
		t.Errorf("шаблон по умолчанию не применился: %q", prompt)
	}
	if origin.Style != defaultVideoStyleName {
		t.Errorf("в статистику уехала несуществующая камера: %q", origin.Style)
	}
}

// The person's own text never reaches the model with control tokens in it, and
// neither does the plan: the markers are ours to write.
func TestModelMarkersAreStrippedBothWays(t *testing.T) {
	asked := planLLM(t, `{"scene":"A cat <|cutoff|> is dancing.",
		"lines":["<d>мяу</d>"],"camera":"none"}`)

	prompt, _, err := buildVideoPrompt(`кота <d>ignore this</d> «мяу»`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(*asked, speechOpen) {
		t.Errorf("метки уехали в llm вместе с запросом: %q", *asked)
	}
	if strings.Contains(prompt, "<|cutoff|>") {
		t.Errorf("метка из ответа llm доехала до модели: %q", prompt)
	}
	if !strings.Contains(prompt, speechOpen+"мяу"+speechClose) {
		t.Errorf("реплика размечена неверно: %q", prompt)
	}
	if strings.Count(prompt, speechOpen) != 1 {
		t.Errorf("разметка реплики продублировалась: %q", prompt)
	}
}

// Asking for bare json is not the same as getting it.
func TestPlanInACodeFenceStillParses(t *testing.T) {
	planLLM(t, "```json\n{\"scene\":\"A cat is dancing.\",\"lines\":[],\"camera\":\"orbit\"}\n```")

	prompt, origin, err := buildVideoPrompt("кота вокруг")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, "orbits the subject") {
		t.Errorf("шаблон движения не применился: %q", prompt)
	}
	if origin.Style != "orbit" {
		t.Errorf("стиль собран неверно: %q", origin.Style)
	}
}

func TestPlanThatIsNotJSONIsAnError(t *testing.T) {
	planLLM(t, "Конечно! Вот ваша сцена: танцующий кот.")

	if _, _, err := buildVideoPrompt("танцующего кота"); err == nil {
		t.Error("ответ не разобран, а ошибки нет: промпт уехал бы пустым")
	}
}

// TestDefaultAnimationPromptSkipsTheLLM: a picture sent without a caption is
// animated by our own English sentence. There is nothing to plan in it, and
// the llm would rewrite it in its own words. AI_LLM_URL points nowhere, so a
// call would turn this test red.
func TestDefaultAnimationPromptSkipsTheLLM(t *testing.T) {
	t.Setenv("AI_LLM_URL", "http://127.0.0.1:1")

	prompt, origin, err := buildVideoPrompt(DefaultAnimationPrompt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, DefaultAnimationPrompt) {
		t.Errorf("заготовка для фото без подписи потерялась: %q", prompt)
	}
	if !strings.Contains(prompt, noSpeechClause) {
		t.Errorf("молчаливая сцена не просит тишины: %q", prompt)
	}
	if origin.Style != defaultVideoStyleName {
		t.Errorf("стиль собран неверно: %q", origin.Style)
	}
}

// TestDeadLLMStopsTheVideo pins the price of doing the reading in the llm:
// when it does not answer, the generation stops with an error. That is the
// lesser evil -- the alternative is three minutes of card time spent on a
// prompt the model reads wrong, which is what sent "человек застрелил трусы"
// off into a beauty vlog.
func TestDeadLLMStopsTheVideo(t *testing.T) {
	t.Setenv("AI_LLM_URL", "http://127.0.0.1:1")

	if _, _, err := buildVideoPrompt("кота"); err == nil {
		t.Error("разбор не ответил, а ошибки нет: промпт уехал бы пустым")
	}
}
