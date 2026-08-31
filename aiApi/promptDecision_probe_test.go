package aiApi

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

// Пробник, а не тест: ходит в живую llm и печатает, что она решила. Ничего не
// утверждает — смотреть глазами. Запуск:
//
//	AI_PROMPT_PROBE=1 go test ./aiApi/ -run TestPromptDecisionProbe -v -timeout 20m
//
// Вопрос, на который он отвечает: если попросить модель саму решать, дополнять
// промпт или оставить как есть, — насколько одинаково она решает это на одном и
// том же входе.

// Кандидаты в системные промпты. Второй отличается тем, что ветка «оставить»
// стоит первой и подана как действие по умолчанию: гипотеза в том, что на
// англоязычном подробном промпте модели просто нечего делать, кроме как
// дополнять, и инструкция «дополни» остаётся единственной живой.
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

// expansionThreshold — во сколько раз ответ должен быть длиннее чистого
// перевода того же промпта, чтобы считать, что модель решила дополнять.
//
// Сравнивать с длиной входа нельзя: перевод с русского на английский сам по
// себе прибавляет четверть слов на одни артикли, и подробный русский промпт
// выглядел бы «дополненным», хотя модель его честно перевела слово в слово.
const expansionThreshold = 1.5

// baselineRuns — сколько раз меряем чистый перевод, чтобы взять медиану: одна
// точка отсчёта сама по себе шумная.
const baselineRuns = 3

// translationBaseline возвращает медианную длину чистого перевода в словах.
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
	// Пробник занимает карту на минуту, поэтому в обычный go test ./... он не
	// лезет — только по явному флагу.
	if os.Getenv("AI_PROMPT_PROBE") == "" {
		t.Skip("пробник выключен; запуск: AI_PROMPT_PROBE=1 go test ./aiApi/ -run TestPromptDecisionProbe -v -timeout 20m")
	}

	if os.Getenv("AI_LLM_URL") == "" {
		// Тесты запускаются не из корня, где лежит .env.local.
		_ = godotenv.Load("../.env.local")
	}
	if os.Getenv("AI_LLM_URL") == "" {
		t.Skip("AI_LLM_URL не задан — пробнику некуда ходить")
	}

	cases := []struct {
		name string
		text string
		want string // чего мы ждём от разумного решения
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
