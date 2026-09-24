package aiApi

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

// A probe, not a test: it goes to the live llm and prints what it decided. It
// asserts nothing; look by eye. Run:
//
//	AI_PROMPT_PROBE=1 go test ./aiApi/ -run TestPromptDecisionProbe -v -timeout 20m
//
// The question it answers: if the model is asked to decide by itself whether to
// expand the prompt or leave it as is, how consistently does it decide on the
// same input.

// Candidate system prompts. The second one differs in that the "leave it"
// branch comes first and is presented as the default action: the hypothesis is
// that on a detailed English prompt the model simply has nothing to do but
// expand, and the "expand" instruction remains the only live one.
var probeSystemPrompts = []struct {
	name   string
	prompt string
}{
	{
		"дополнение первым",
		"Ты готовишь промпт для генератора изображений. Верни промпт на английском. " +
			"Если описание пользователя скудное, дополни его визуальными деталями: обстановка, свет, композиция. " +
			"Если описание уже подробное, ничего не добавляй и не убирай, только переведи на английский. " +
			"Не добавляй стилевые и качественные слова вроде anime, photorealistic, 8k, masterpiece. " +
			"Формат вывода — только промпт.",
	},
	{
		"сохранение первым",
		"Ты готовишь промпт для генератора изображений. Верни промпт на английском. " +
			"По умолчанию ничего не выдумывай: если пользователь уже назвал сцену, объекты, свет или обстановку, " +
			"верни его текст как есть, только переведи на английский. Если он уже на английском и подробный — верни его дословно. " +
			"Дополняй визуальными деталями только тогда, когда описание состоит из одного-двух объектов и обстановки в нём нет вовсе. " +
			"Не добавляй стилевые и качественные слова вроде anime, photorealistic, 8k, masterpiece. " +
			"Формат вывода — только промпт.",
	},
}

const probeRepeats = 5

// expansionThreshold is how many times longer than a plain translation of the
// same prompt the answer must be to count as the model deciding to expand.
//
// Comparing with the input length won't do: translating from Russian to English
// adds a quarter more words on articles alone, and a detailed Russian prompt
// would look "expanded" even though the model honestly translated it word for
// word.
const expansionThreshold = 1.5

// baselineRuns is how many times the plain translation is measured to take the
// median: a single reference point is noisy on its own.
const baselineRuns = 3

// translationBaseline returns the median length of a plain translation in
// words.
func translationBaseline(t *testing.T, text string) int {
	t.Helper()

	lengths := []int{}
	for i := 0; i < baselineRuns; i++ {
		got, err := askLLM(translateSystemPrompt, text)
		if err != nil {
			t.Errorf("baseline for %q: %v", text, err)
			continue
		}
		lengths = append(lengths, len(strings.Fields(got)))
	}

	if len(lengths) == 0 {
		return len(strings.Fields(text))
	}

	sort.Ints(lengths)
	return lengths[len(lengths)/2]
}

func TestPromptDecisionProbe(t *testing.T) {
	// The probe occupies the GPU for a minute, so it stays out of a regular go
	// test ./... and runs only with an explicit flag.
	if os.Getenv("AI_PROMPT_PROBE") == "" {
		t.Skip("пробник выключен; запуск: AI_PROMPT_PROBE=1 go test ./aiApi/ -run TestPromptDecisionProbe -v -timeout 20m")
	}

	if os.Getenv("AI_LLM_URL") == "" {
		// Tests run not from the root, where .env.local lives.
		_ = godotenv.Load("../.env.local")
	}
	if os.Getenv("AI_LLM_URL") == "" {
		t.Skip("AI_LLM_URL не задан — пробнику некуда ходить")
	}

	cases := []struct {
		name string
		text string
		want string // what we expect from a sensible decision
	}{
		{"одно слово", "кот", "дополнить"},
		{"одно слово по-английски", "cat", "дополнить"},
		{"скудный запрос", "кот в скафандре", "дополнить"},
		{
			"подробный запрос",
			"рыжий кот сидит на подоконнике старой хрущёвки, за окном идёт снег, тёплый свет настольной лампы падает слева, на стекле капли",
			"оставить",
		},
		{
			"подробный запрос по-английски",
			"a ginger cat on the windowsill of an old apartment, snow falling outside, warm desk lamp light from the left, water drops on the glass",
			"оставить",
		},
		{
			"средний запрос",
			"кот на подоконнике зимой",
			"на усмотрение",
		},
	}

	for _, c := range cases {
		inputWords := len(strings.Fields(c.text))
		baseline := translationBaseline(t, c.text)

		t.Logf("\n=== %s (%d слов, чистый перевод %d слов, ожидаем: %s)\n    вход: %s",
			c.name, inputWords, baseline, c.want, c.text)

		for _, variant := range probeSystemPrompts {
			decisions := make(map[string]int)

			for run := 1; run <= probeRepeats; run++ {
				got, err := askLLM(variant.prompt, c.text)
				if err != nil {
					t.Errorf("%s / %s, прогон %d: %v", c.name, variant.name, run, err)
					continue
				}

				outputWords := len(strings.Fields(got))
				ratio := float64(outputWords) / float64(max(baseline, 1))

				decision := "оставить"
				if ratio >= expansionThreshold {
					decision = "дополнить"
				}
				decisions[decision]++
			}

			verdict := "СТАБИЛЬНО"
			if len(decisions) > 1 {
				verdict = "ПЛАВАЕТ"
			}
			t.Logf("    [%-18s] %-9s %v", variant.name, verdict, decisions)
		}
	}
}
