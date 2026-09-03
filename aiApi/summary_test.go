package aiApi

import (
	"os"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

func TestSummarySystemPromptNamesTheLanguage(t *testing.T) {
	cases := [][2]string{
		{"ru", "русском языке"},
		{"de", "немецком языке"},
		{"he", "иврите"},
		{"hi", "хинди"},
		{"pt-BR", "португальском языке"},
		{"EN", "английском языке"},
	}

	for _, c := range cases {
		prompt := summarySystemPrompt(c[0])

		if !strings.Contains(prompt, c[1]) {
			t.Errorf("summarySystemPrompt(%q): нет %q в %s", c[0], c[1], prompt)
		}
		// Сам код в промпт попадать не должен: расшифровывать его модель
		// не обязана, за неё это уже сделали.
		if strings.Contains(prompt, "IETF") {
			t.Errorf("summarySystemPrompt(%q) всё ещё просит разобрать код: %s", c[0], prompt)
		}
	}
}

func TestSummarySystemPromptFallsBack(t *testing.T) {
	// Telegram присылает language_code не всегда, а код может оказаться и
	// незнакомым — тогда язык берём из самого текста.
	for _, code := range []string{"", "  ", "xx", "klingon"} {
		prompt := summarySystemPrompt(code)

		if !strings.Contains(prompt, "языке самого текста") {
			t.Errorf("summarySystemPrompt(%q): нет запасной инструкции: %s", code, prompt)
		}
	}
}

// Пробник: проверяет на живой llm, что пересказ выходит на языке из
// language_code. Остальное тут детерминированно, а вот язык — единственное,
// что реально зависит от модели.
//
//	AI_SUMMARY_PROBE=1 go test ./aiApi/ -run TestSummaryLanguageProbe -v -timeout 10m
func TestSummaryLanguageProbe(t *testing.T) {
	if os.Getenv("AI_SUMMARY_PROBE") == "" {
		t.Skip("пробник выключен; запуск: AI_SUMMARY_PROBE=1 go test ./aiApi/ -run TestSummaryLanguageProbe -v -timeout 10m")
	}
	if os.Getenv("AI_LLM_URL") == "" {
		_ = godotenv.Load("../.env.local")
	}
	if os.Getenv("AI_LLM_URL") == "" {
		t.Skip("AI_LLM_URL не задан")
	}

	const source = "Вчера на совещании обсуждали переезд серверов в новый дата-центр. " +
		"Решили переносить по очереди, начиная с тестового контура, чтобы не ронять прод. " +
		"Сроки сдвинули на две недели, потому что поставщик задерживает стойки. " +
		"Ответственным назначили отдел эксплуатации, отчитываться раз в неделю по пятницам."

	for _, code := range []string{"ru", "en", "de", ""} {
		summary, err := generateSummary(source, code)
		if err != nil {
			t.Errorf("код %q: %v", code, err)
			continue
		}
		t.Logf("[%s] %s", code, summary)
	}
}
