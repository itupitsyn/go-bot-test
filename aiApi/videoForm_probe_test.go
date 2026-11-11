package aiApi

import (
	"os"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

// A probe, not a test: it takes the живой llm and the real assembly and prints
// what comes out. It asserts almost nothing; look by eye. Run:
//
//	AI_VIDEOFORM_PROBE=1 go test ./aiApi/ -run TestVideoFormProbe -v -timeout 20m
//
// What it is for. The forms were measured on a python copy of the prompt, on
// the 43 requests with speech in the journal; this walks the SAME ground
// through the code that actually ships, so that a difference between the two
// shows up here and not in production.
//
// The cases below are the ones that decided the design on 11.11.2026:
// a forwarded news item (narrator), a request that is somebody's own line
// (speaks), two people (dialogue), a plain description (silent), and a
// retelling with indirect speech, which used to come out as the person in the
// shot reading a news item about himself.
var videoFormCases = []struct {
	name string
	text string
	want string
}{
	{"новость про Дубай", "Власти Дубая разрабатывают ПДД для летающих " +
		"автомобилей, сообщил газете «Имарат аль-Яум» руководитель " +
		"подразделения по лицензированию Ахмед Махбуб.", formNarrator},
	{"своя реплика в кавычках",
		`женщина поворачивается и говорит: «Приказываю тебе ка-кать!»`, formSpeaks},
	{"двое говорят",
		`двое на кухне, первый: «Ты где был?», второй: «На работе»`, formDialogue},
	{"описание сцены", "батька решает вопросы с помощью морковки", formSilent},
	{"пересказ с косвенной речью", "Дзермант заявил, что в местном варианте " +
		"русского языка правильным является написание Беларусь.", formNarrator},
}

func TestVideoFormProbe(t *testing.T) {
	if os.Getenv("AI_VIDEOFORM_PROBE") == "" {
		t.Skip("AI_VIDEOFORM_PROBE не задан")
	}
	if os.Getenv("AI_LLM_URL") == "" {
		_ = godotenv.Load("../.env.local")
	}
	if os.Getenv("AI_LLM_URL") == "" {
		t.Skip("AI_LLM_URL не задан — пробнику некуда ходить")
	}

	for _, c := range videoFormCases {
		plan, err := planVideo(c.text, 5)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}

		prompt, _, err := buildVideoPrompt(c.text, 5)
		if err != nil {
			t.Errorf("%s: сборка: %v", c.name, err)
			continue
		}

		mark := "  "
		if plan.Form != c.want {
			mark = "!!"
		}
		t.Logf("%s %-26s форма=%-8s (ждали %s), реплик=%d, блоков=%d",
			mark, c.name, plan.Form, c.want, len(plan.Lines),
			strings.Count(prompt, speechOpen))
		t.Logf("      %s", prompt)

		// Единственное, на чём пробник настаивает: диктор говорит одним
		// голосом. Это не вкус, это замер — две пары маркеров дают двух
		// разных говорящих.
		if plan.Form == formNarrator && strings.Count(prompt, speechOpen) > 1 {
			t.Errorf("%s: диктора разделили на %d голосов",
				c.name, strings.Count(prompt, speechOpen))
		}
	}
}
