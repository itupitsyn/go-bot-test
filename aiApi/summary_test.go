package aiApi

import (
	"os"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

func TestSummarySystemPromptCarriesLanguage(t *testing.T) {
	for _, code := range []string{"ru", "en", "pt-BR"} {
		prompt := summarySystemPrompt(code)

		if !strings.Contains(prompt, code) {
			t.Errorf("summarySystemPrompt(%q) does not mention the code: %s", code, prompt)
		}
		if !strings.Contains(prompt, "языке самого текста") {
			t.Errorf("summarySystemPrompt(%q) drops the fallback for an unknown code", code)
		}
	}
}

func TestSummarySystemPromptWithoutLanguage(t *testing.T) {
	// Telegram присылает language_code не всегда — тогда про код речи быть
	// не должно вовсе, иначе модель получит инструкцию про пустую строку.
	prompt := summarySystemPrompt("")

	if strings.Contains(prompt, "IETF") {
		t.Errorf("пустой код не должен попадать в промпт: %s", prompt)
	}
	if !strings.Contains(prompt, "языке самого текста") {
		t.Errorf("нет запасной инструкции про язык: %s", prompt)
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
