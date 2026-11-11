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

// TestCameraAndSoundAreAlwaysThere is what the six templates used to
// guarantee for free. H3 writes the soundtrack in the same pass whether anyone
// asked or not, so a prompt that says nothing about sound leaves it to chance
// — and a prompt that says nothing at all has been heard inventing English
// dialogue over a Russian scene.
func TestCameraAndSoundAreAlwaysThere(t *testing.T) {
	planLLM(t, `{"form":"silent","scene":"A cat is dancing","lines":[],
		"camera":"","sound":""}`)

	prompt, origin, err := buildVideoPrompt("танцующего кота", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(strings.ToLower(prompt), defaultCamera) {
		t.Errorf("движение камеры потерялось: %q", prompt)
	}
	if !strings.Contains(strings.ToLower(prompt), defaultSound) {
		t.Errorf("про звук промолчали: %q", prompt)
	}
	if strings.Contains(origin.Style, speechStyleMark) {
		t.Errorf("в журнал уехало %q, ждали слово движения", origin.Style)
	}
}

// TestCameraWordIsReadBackForTheJournal: the column used to hold the name of
// the chosen template, and that is how we know "pull-back" is on 48 clips of
// 162. The templates are gone, so the word is read back out of the phrase.
func TestCameraWordIsReadBackForTheJournal(t *testing.T) {
	for phrase, want := range map[string]string{
		"a slow push-in until her eyes fill the frame": "push-in",
		"the camera pulls back to reveal the kitchen":  "pull-back",
		"a sweeping aerial shot over the city":         "aerial",
		"the camera drifts sideways past the window":   "free",
	} {
		if got := cameraWord(phrase); got != want {
			t.Errorf("из %q вышло %q, ждали %q", phrase, got, want)
		}
	}
}

// TestBuildVideoPromptFollowsThePlan is the whole contract in one test: the
// scene comes back in English, the line in the language it was typed in, and
// the camera chooses the template.
func TestBuildVideoPromptFollowsThePlan(t *testing.T) {
	asked := planLLM(t, `{"form":"speaks",
		"scene":"A man is in a subway station and then he says:",
		"lines":["я опоздал"],"camera":"a slow push-in until her eyes fill the frame",
		"sound":"street noise"}`)

	prompt, origin, err := buildVideoPrompt(`мужика в метро «я опоздал» медленно`, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(*asked, "мужика в метро") {
		t.Errorf("на разбор уехало не то: %q", *asked)
	}

	want := "A man is in a subway station and then he says: " +
		speechOpen + "я опоздал" + speechClose +
		". A slow push-in until her eyes fill the frame. Street noise."
	if !strings.Contains(prompt, want) {
		t.Errorf("промпт собран неверно: %q", prompt)
	}

	if strings.Contains(prompt, noSpeechClause) {
		t.Errorf("просим и говорить, и молчать одновременно: %q", prompt)
	}

	if origin.Source != `мужика в метро «я опоздал» медленно` {
		t.Errorf("исходный запрос потерялся: %+v", origin)
	}
	// Форма едет в журнал вместе со стилем: иначе потом не узнать, как часто
	// просят диктора, а как часто говорящего в кадре.
	if origin.Style != "push-in"+speechStyleMark+":"+formSpeaks {
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
		`человек стреляет в трусы, а потом говорит: "Я покакал"`, 5)
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
	if camera := strings.Index(prompt, "the camera"); camera >= 0 && camera < line {
		t.Errorf("движение камеры стоит там, где модель ждёт реплику: %q", prompt)
	}
}

func TestSceneRequestAsksForSilence(t *testing.T) {
	planLLM(t, `{"scene":"A cat is dancing.","lines":[],"camera":"none"}`)

	prompt, origin, err := buildVideoPrompt("танцующего кота", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(prompt, speechOpen) {
		t.Errorf("в молчаливой сцене появилась разметка реплики: %q", prompt)
	}
	if !strings.Contains(prompt, noSpeechClause) {
		t.Errorf("сцена не просит тишины, а это и есть лечение зачитывания: %q", prompt)
	}
	if strings.Contains(origin.Style, speechStyleMark) {
		t.Errorf("сцене приписали речь: %q", origin.Style)
	}
}

// TestLineOnlyRequestGetsASpeaker: the llm returns an empty scene for a request
// that is nothing but a line, and the template would wrap emptiness.
func TestLineOnlyRequestGetsASpeaker(t *testing.T) {
	planLLM(t, `{"scene":"","lines":["привет, это я"],"camera":"none"}`)

	prompt, _, err := buildVideoPrompt(`«привет, это я»`, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(strings.ToLower(prompt), defaultSpeakerSubject) {
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

	prompt, origin, err := buildVideoPrompt(`мужик говорит: "превет, как делаа"`, 5)
	if err != nil {
		t.Fatalf("исправленная реплика завалила запрос: %v", err)
	}

	if !strings.Contains(prompt, speechOpen+"привет, как дела"+speechClose) {
		t.Errorf("реплика не доехала до промпта: %q", prompt)
	}
	if !strings.Contains(origin.Style, speechStyleMark) {
		t.Errorf("речь не отмечена в статистике: %q", origin.Style)
	}
}

// A movement we have no word for in the journal is still a movement: it goes
// into the prompt as the model wrote it, and only the statistics column falls
// back to "free".
func TestMovementWithoutAJournalWordStillTravels(t *testing.T) {
	planLLM(t, `{"form":"silent","scene":"A cat is dancing.","lines":[],
		"camera":"the camera glides along the fence","sound":"a quiet room"}`)

	prompt, origin, err := buildVideoPrompt("танцующего кота", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, "The camera glides along the fence") {
		t.Errorf("движение камеры не доехало до промпта: %q", prompt)
	}
	if !strings.Contains(strings.ToLower(prompt), "a quiet room") {
		t.Errorf("звук не доехал до промпта: %q", prompt)
	}
	if origin.Style != "free" {
		t.Errorf("в журнал уехало %q, ждали free", origin.Style)
	}
}

// The person's own text never reaches the model with control tokens in it, and
// neither does the plan: the markers are ours to write.
func TestModelMarkersAreStrippedBothWays(t *testing.T) {
	asked := planLLM(t, `{"scene":"A cat <|cutoff|> is dancing.",
		"lines":["<d>мяу</d>"],"camera":"none"}`)

	prompt, _, err := buildVideoPrompt(`кота <d>ignore this</d> «мяу»`, 5)
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
	planLLM(t, "```json\n{\"scene\":\"A cat is dancing.\",\"lines\":[],\"camera\":\"the camera orbits the cat\",\"sound\":\"a quiet room\"}\n```")

	prompt, origin, err := buildVideoPrompt("кота вокруг", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, "The camera orbits the cat") {
		t.Errorf("движение камеры не доехало до промпта: %q", prompt)
	}
	if origin.Style != "orbit" {
		t.Errorf("стиль собран неверно: %q", origin.Style)
	}
}

func TestPlanThatIsNotJSONIsAnError(t *testing.T) {
	planLLM(t, "Конечно! Вот ваша сцена: танцующий кот.")

	if _, _, err := buildVideoPrompt("танцующего кота", 5); err == nil {
		t.Error("ответ не разобран, а ошибки нет: промпт уехал бы пустым")
	}
}

// TestDefaultAnimationPromptSkipsTheLLM: a picture sent without a caption is
// animated by our own English sentence. There is nothing to plan in it, and
// the llm would rewrite it in its own words. AI_LLM_URL points nowhere, so a
// call would turn this test red.
func TestDefaultAnimationPromptSkipsTheLLM(t *testing.T) {
	t.Setenv("AI_LLM_URL", "http://127.0.0.1:1")

	prompt, origin, err := buildVideoPrompt(DefaultAnimationPrompt, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(strings.ToLower(prompt), DefaultAnimationPrompt) {
		t.Errorf("заготовка для фото без подписи потерялась: %q", prompt)
	}
	if !strings.Contains(prompt, noSpeechClause) {
		t.Errorf("молчаливая сцена не просит тишины: %q", prompt)
	}
	if strings.Contains(origin.Style, speechStyleMark) {
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

	if _, _, err := buildVideoPrompt("кота", 5); err == nil {
		t.Error("разбор не ответил, а ошибки нет: промпт уехал бы пустым")
	}
}

// TestDescriptionIsNotRecitedAloud is the bug measured on 26.10.2026: the plan
// pulled a fragment of a plain description into "lines", and the person got a
// clip of someone pronouncing the word «крякает» instead of a tomato quacking.
// Nothing in the request says anything is spoken, and what the plan quoted is
// not even the whole of it.
func TestDescriptionIsNotRecitedAloud(t *testing.T) {
	planLLM(t, `{"scene":"A tomato stands on a surface, and then it says:",
		"lines":["крякает дуплом"],"camera":"none"}`)

	prompt, origin, err := buildVideoPrompt("томат крякает дуплом", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(prompt, speechOpen) {
		t.Errorf("описание уехало репликой: %q", prompt)
	}
	if !strings.Contains(prompt, noSpeechClause) {
		t.Errorf("реплику убрали, а тишины не попросили: %q", prompt)
	}
	if strings.Contains(strings.ToLower(prompt), "and then") {
		t.Errorf("сцена осталась с передачей слова, а говорить уже некому: %q", prompt)
	}
	if strings.Contains(origin.Style, speechStyleMark) {
		t.Errorf("речь отметилась в статистике, хотя её нет: %q", origin.Style)
	}
}

// TestBareRequestThatIsTheWholeLineStillSpeaks is the other side of the same
// check, and the reason it is not simply "a line must come from quotes".
// People do mean speech without marking it: this one arrived bare, and what
// makes it a line is that it IS the request, whole, with nothing left over to
// film. The spelling is allowed to differ -- a full stop is not a different
// sentence.
func TestBareRequestThatIsTheWholeLineStillSpeaks(t *testing.T) {
	planLLM(t, `{"scene":"","lines":["Но не помним почему."],"camera":"none"}`)

	prompt, origin, err := buildVideoPrompt("но не помним почему", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, speechOpen+"Но не помним почему."+speechClose) {
		t.Errorf("задуманная реплика потерялась: %q", prompt)
	}
	if !strings.Contains(origin.Style, speechStyleMark) {
		t.Errorf("речь не отмечена в статистике: %q", origin.Style)
	}
}

// A verb of speaking is enough on its own: the line may then be a fragment of
// the request, because the rest of it is the scene.
func TestIntroducedSpeechKeepsAFragment(t *testing.T) {
	planLLM(t, `{"scene":"An old man stands in a field and then he says:",
		"lines":["ура"],"camera":"none"}`)

	prompt, _, err := buildVideoPrompt("дед кричит ура", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, speechOpen+"ура"+speechClose) {
		t.Errorf("введённая глаголом реплика потерялась: %q", prompt)
	}
}

func TestTrimSpeechIntro(t *testing.T) {
	cases := map[string]string{
		"Zelensky is eating food, and then he says:":       "Zelensky is eating food",
		"Donald Trump walks into the frame and then says:": "Donald Trump walks into the frame",
		"A person stands in a kitchen while saying:":       "A person stands in a kitchen",
		"A goose walks into the frame and defecates.":      "A goose walks into the frame and defecates.",
		"A tomato sits on a surface:":                      "A tomato sits on a surface",
	}

	for scene, want := range cases {
		if got := trimSpeechIntro(scene); got != want {
			t.Errorf("trimSpeechIntro(%q) = %q, ожидалось %q", scene, got, want)
		}
	}
}

// TestNarratorReadsInOneBlock is the rule that came out of the clips of
// 11.11.2026: a block per line is a SPEAKER per line, and two blocks were read
// by two different voices. A news item split across two blocks would be read
// by two strangers taking turns, so the voice-over goes into one block however
// many sentences it has.
func TestNarratorReadsInOneBlock(t *testing.T) {
	planLLM(t, `{"form":"narrator",
		"scene":"A city street with electric scooters. Nobody in the shot speaks. An unseen narrator tells over the footage:",
		"lines":["Власти города утвердили правила.","Документ вступит в силу в декабре."],
		"camera":"none"}`)

	prompt, origin, err := buildVideoPrompt("власти утвердили правила для самокатов", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Count(prompt, speechOpen) != 1 {
		t.Errorf("диктора разделили на два голоса: %q", prompt)
	}
	want := speechOpen + "Власти города утвердили правила. Документ вступит в силу в декабре." + speechClose
	if !strings.Contains(prompt, want) {
		t.Errorf("текст диктора собран неверно: %q", prompt)
	}
	if !strings.Contains(origin.Style, formNarrator) {
		t.Errorf("форма не доехала до статистики: %q", origin.Style)
	}
}

// TestDialogueKeepsAVoicePerLine is the same rule from the other side.
func TestDialogueKeepsAVoicePerLine(t *testing.T) {
	planLLM(t, `{"form":"dialogue",
		"scene":"Two men face each other in a kitchen, and then they say:",
		"lines":["Ты где был?","На работе"],"camera":"none"}`)

	prompt, _, err := buildVideoPrompt(`двое на кухне: «Ты где был?» «На работе»`, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Count(prompt, speechOpen) != 2 {
		t.Errorf("диалог склеили в один голос: %q", prompt)
	}
}

// TestUnknownFormIsReadFromTheLines: a form we do not know is not worth
// failing over. The lines themselves say whether anyone speaks, and that
// reading is the one the service lived on before the forms existed.
func TestUnknownFormIsReadFromTheLines(t *testing.T) {
	for _, c := range []struct {
		lines []string
		want  string
	}{
		{nil, formSilent},
		{[]string{"одна"}, formSpeaks},
		{[]string{"одна", "вторая"}, formDialogue},
	} {
		if got := knownForm("сочинённая форма", c.lines); got != c.want {
			t.Errorf("на %d репликах форма %q, ждали %q", len(c.lines), got, c.want)
		}
	}
}

// TestBudgetTravelsWithTheRequest: until 11.11.2026 the bot cut "10 секунд"
// out of the text and kept the number to itself, so the plan wrote a
// voice-over for a clip whose length it did not know. Three words a second is
// measured, not assumed — see wordsPerSecond.
func TestBudgetTravelsWithTheRequest(t *testing.T) {
	asked := planLLM(t, `{"form":"silent","scene":"A cat","lines":[],"camera":"none"}`)

	if _, _, err := buildVideoPrompt("кота", 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(*asked, "30 слов") {
		t.Errorf("бюджет на десять секунд не доехал до модели: %s", *asked)
	}
	if !strings.Contains(*asked, "10 с") {
		t.Errorf("длина ролика не доехала до модели: %s", *asked)
	}
}

// TestBudgetFallsBackToFiveSeconds: zero means the person asked for no
// particular length, and the service then makes five seconds.
func TestBudgetFallsBackToFiveSeconds(t *testing.T) {
	if got := wordBudget(0); got != 15 {
		t.Errorf("бюджет по умолчанию %d слов, ждали 15", got)
	}
	if got := wordBudget(15); got != 45 {
		t.Errorf("бюджет на пятнадцать секунд %d слов, ждали 45", got)
	}
}

// TestTranslatedNarrationFallsBackToTheRequest. The prompt says never to
// translate what gets said out loud, and the rule floats: on 11.11.2026 the
// same news item came back in Russian on one run and in English on the next.
// A Russian-speaking person would have got an English voice-over over their
// own news item.
func TestTranslatedNarrationFallsBackToTheRequest(t *testing.T) {
	planLLM(t, `{"form":"narrator",
		"scene":"A city street. Nobody in the shot speaks. An unseen narrator tells over the footage:",
		"lines":["Dubai authorities develop traffic rules for flying cars."],
		"camera":"a slow pan","sound":"street noise"}`)

	request := "Власти Дубая разрабатывают ПДД для летающих автомобилей"
	prompt, _, err := buildVideoPrompt(request, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, speechOpen+request+speechClose) {
		t.Errorf("перевод не заменён словами человека: %q", prompt)
	}
	if strings.Contains(prompt, "Dubai authorities develop") {
		t.Errorf("английский перевод остался в реплике: %q", prompt)
	}
}

// TestEnglishRequestKeepsItsEnglishLine is the other side: the guard must not
// fire when the person wrote in English themselves.
func TestEnglishRequestKeepsItsEnglishLine(t *testing.T) {
	planLLM(t, `{"form":"speaks","scene":"A man waves and then he says:",
		"lines":["hello there"],"camera":"a static shot","sound":"a quiet room"}`)

	prompt, _, err := buildVideoPrompt(`a man waving, he says "hello there"`, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, speechOpen+"hello there"+speechClose) {
		t.Errorf("английскую реплику у английской просьбы отобрали: %q", prompt)
	}
}

// TestTranslatedLineInShotIsOnlyLogged: for a line said in shot the original
// sits somewhere inside the request, and digging it out by pattern is the sort
// of guessing this feature was moved away from. The line stands; the log says
// what happened.
func TestTranslatedLineInShotIsOnlyLogged(t *testing.T) {
	planLLM(t, `{"form":"speaks","scene":"A woman turns and then she says:",
		"lines":["I order you to poop"],"camera":"a push-in","sound":"a quiet room"}`)

	prompt, _, err := buildVideoPrompt(`женщина говорит: «Приказываю тебе какать»`, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, speechOpen+"I order you to poop"+speechClose) {
		t.Errorf("реплика в кадре пропала вместо того, чтобы остаться: %q", prompt)
	}
}

// TestTranslationFallbackStaysInsideTheBudget is the case where the two
// repairs meet: the model shortens the retelling AND translates it, the guard
// puts the whole request back, and the shortening is undone. Left alone, a
// request a little over budget lands in the band where speech GARBLES — far
// over it truncates cleanly, so the dangerous length is the near one.
func TestTranslationFallbackStaysInsideTheBudget(t *testing.T) {
	long := "Власти города сегодня утвердили новые правила движения для " +
		"электрических самокатов в центральной части города, сообщает " +
		"пресс-служба мэрии, документ вступит в силу с первого декабря"
	planLLM(t, `{"form":"narrator",
		"scene":"A city street. Nobody in the shot speaks. An unseen narrator tells over the footage:",
		"lines":["City authorities approved new scooter rules."],
		"camera":"a slow pan","sound":"street noise"}`)

	prompt, _, err := buildVideoPrompt(long, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	said := prompt[strings.Index(prompt, speechOpen)+len(speechOpen):]
	said = said[:strings.Index(said, speechClose)]
	if words := len(strings.Fields(said)); words != wordBudget(5) {
		t.Errorf("диктору осталось %d слов при бюджете %d: %q",
			words, wordBudget(5), said)
	}
	if !strings.HasPrefix(said, "Власти города сегодня") {
		t.Errorf("вернули не слова человека: %q", said)
	}
}

// TestNarrationThatFitsIsLeftAlone: the backstop must not touch text that the
// model already shortened properly. Cutting at a word boundary is brutal next
// to a retelling done by meaning.
func TestNarrationThatFitsIsLeftAlone(t *testing.T) {
	planLLM(t, `{"form":"narrator",
		"scene":"A city street. Nobody in the shot speaks. An unseen narrator tells over the footage:",
		"lines":["Город утвердил правила для самокатов."],
		"camera":"a slow pan","sound":"street noise"}`)

	prompt, _, err := buildVideoPrompt("длинная новость про самокаты и правила", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, speechOpen+"Город утвердил правила для самокатов."+speechClose) {
		t.Errorf("короткий пересказ порезали зря: %q", prompt)
	}
}

// TestLongerClipKeepsMoreWords ties the backstop to the measured law: three
// words a second, linear in the length.
func TestLongerClipKeepsMoreWords(t *testing.T) {
	words := make([]string, 60)
	for i := range words {
		words[i] = "слово"
	}
	long := strings.Join(words, " ")

	if got := len(clipToBudget([]string{long}, wordBudget(5))); got != 1 {
		t.Fatalf("после обрезки осталось %d строк, ждали одну", got)
	}
	five := strings.Fields(clipToBudget([]string{long}, wordBudget(5))[0])
	fifteen := strings.Fields(clipToBudget([]string{long}, wordBudget(15))[0])
	if len(five) != 15 || len(fifteen) != 45 {
		t.Errorf("на 5 с осталось %d слов, на 15 с — %d; ждали 15 и 45",
			len(five), len(fifteen))
	}
}

// TestDialogueSceneIsNotHandedToANarrator is a production case of 11.11.2026,
// reported by the owner: a lightsaber duel between two named people. The form
// came back "dialogue" — two shouted lines, one each — and the scene
// nevertheless ended with "then an unseen narrator tells over the footage:".
// H3 was asked to read two shouted lines over an epic battle in a voice
// belonging to nobody on screen, and delivered them half sung.
func TestDialogueSceneIsNotHandedToANarrator(t *testing.T) {
	planLLM(t, `{"form":"dialogue",
		"scene":"Two men duel with lightsabers over lava. They fight on, then an unseen narrator tells over the footage:",
		"lines":["Ты был мне как брат","Ненавижу!"],
		"camera":"a tracking shot","sound":"the hum of blades"}`)

	prompt, _, err := buildVideoPrompt(`дуэль на мечах, первый кричит «Ты был мне как брат», второй кричит «Ненавижу!»`, 15)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(strings.ToLower(prompt), "narrator") {
		t.Errorf("речь в кадре отдали диктору: %q", prompt)
	}
	if !strings.Contains(prompt, speakerIntro+" "+speechOpen) {
		t.Errorf("сцена не передаёт слово говорящим: %q", prompt)
	}
	if strings.Count(prompt, speechOpen) != 2 {
		t.Errorf("двух говорящих склеили: %q", prompt)
	}
}

// TestNarratorKeepsItsOwnIntro is the other side: the repair must not fire on
// the form the phrase belongs to.
func TestNarratorKeepsItsOwnIntro(t *testing.T) {
	planLLM(t, `{"form":"narrator",
		"scene":"A city street. Nobody in the shot speaks. An unseen narrator tells over the footage:",
		"lines":["Город утвердил правила"],"camera":"a slow pan","sound":"street noise"}`)

	prompt, _, err := buildVideoPrompt("город утвердил правила для самокатов", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(strings.ToLower(prompt), "unseen narrator") {
		t.Errorf("у диктора отобрали его собственную подводку: %q", prompt)
	}
}

// TestSpeaksSceneKeepsAProperIntro: the repair cuts only the narrator clause,
// and leaves a scene that already hands over correctly alone.
func TestSpeaksSceneKeepsAProperIntro(t *testing.T) {
	planLLM(t, `{"form":"speaks","scene":"A woman turns to the camera and then she says:",
		"lines":["привет"],"camera":"a push-in","sound":"a quiet room"}`)

	prompt, _, err := buildVideoPrompt(`женщина говорит «привет»`, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(prompt, "and then she says: "+speechOpen) {
		t.Errorf("правильную подводку испортили: %q", prompt)
	}
}
