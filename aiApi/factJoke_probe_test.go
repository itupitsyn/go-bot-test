package aiApi

import (
	"os"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

// A probe, not a test: it goes to the live llm and prints the jokes it got, so
// a model or a quant change can be judged by what the chat would actually see.
//
//	AI_JOKE_PROBE=1 go test ./aiApi/ -run TestFactJokeProbe -v -timeout 20m -count=1
//
// -count=1 matters: go caches test results, and a cached pass would silently
// replay the previous model's answers.
//
// Two numbers come out of it, and both are about shape rather than humour,
// because whether a joke is funny is beyond rules (see jokeProblem):
//
//   - dropped: the message got no joke at all, which means all three variants
//     failed the gate (empty, too long, a repeated word, garbage or a copy of
//     an example). A quant that reasons worse tends to raise this.
//   - openers: how many jokes start with the same stem. The feature exists
//     because of this: without the recent jokes in the prompt 14 of 25 opened
//     with "Проверил", with them 5 (see factJoke.go).
//
// Messages are the kinds that have nothing to check: moods, opinions, requests,
// small talk — the exact traffic that lands in this branch.
func TestFactJokeProbe(t *testing.T) {
	if os.Getenv("AI_JOKE_PROBE") == "" {
		t.Skip("пробник выключен; запуск: AI_JOKE_PROBE=1 go test ./aiApi/ -run TestFactJokeProbe -v -timeout 20m -count=1")
	}
	if os.Getenv("AI_LLM_URL") == "" {
		_ = godotenv.Load("../.env.local")
	}
	if os.Getenv("AI_LLM_URL") == "" {
		t.Skip("AI_LLM_URL не задан — пробнику некуда ходить")
	}

	messages := []string{
		"доброе утро",
		"я сегодня король мира",
		"кажется, я заболел",
		"у нас лучшая команда",
		"закажите кто-нибудь пиццу",
		"опять дождь",
		"я выспался впервые за неделю",
		"этот сериал лучший в истории",
		"не могу найти зарядку",
		"кто за кофе?",
		"мне кажется, начальник меня недолюбливает",
		"ну такое себе",
	}

	// The chat's last jokes are fed back, exactly as the bot does it.
	const recentKept = 5
	var recent []string
	openers := map[string]int{}
	dropped := 0

	for _, message := range messages {
		joke, err := generateFactJoke(message, recent)
		if err != nil {
			t.Errorf("%q: %v", message, err)
			continue
		}
		if joke == "" {
			dropped++
			t.Logf("%-42s → (ни одна из трёх не прошла привратника)", message)
			continue
		}

		openers[jokeOpener(joke)]++
		recent = append(recent, joke)
		if len(recent) > recentKept {
			recent = recent[len(recent)-recentKept:]
		}
		t.Logf("%-42s → %s", message, joke)
	}

	repeated := 0
	var worst string
	for opener, n := range openers {
		if n > 1 {
			repeated += n - 1
			if worst == "" || n > openers[worst] {
				worst = opener
			}
		}
	}

	t.Logf("")
	t.Logf("сообщений %d | без шутки %d | повторных зачинов %d",
		len(messages), dropped, repeated)
	if worst != "" {
		t.Logf("самый частый зачин: «%s» — %d раз", worst, openers[worst])
	}
	t.Logf("разных зачинов: %d", len(openers))
	if strings.TrimSpace(worst) == "" && len(openers) == 0 {
		t.Error("ни одной шутки не получено — проверь адрес модели")
	}
}
