package aiApi

import (
	"os"
	"testing"
	"time"
)

// A probe, not a test: it goes to the live llm and the live search and prints
// what it decided. Run with both addresses in the environment:
//
//	AI_FACTCHECK_PROBE=1 AI_LLM_URL=... FACTCHECK_SEARCH_URL=... go test ./aiApi/ -run TestFactCheckProbe -v -timeout 20m
//
// It is here to see that the Go chain behaves like the Python one it was
// measured with: "not found" on silence, no confident nonsense.
func TestFactCheckProbe(t *testing.T) {
	if os.Getenv("AI_FACTCHECK_PROBE") == "" {
		t.Skip("probe: set AI_FACTCHECK_PROBE=1")
	}

	claims := []struct{ claim, expected string }{
		{"James Talarico defeated Ken Paxton in the 2026 Texas US Senate race.", "confirmed"},
		{"Кен Пакстон выиграл выборы в Сенат от Техаса в 2026 году.", "refuted"},
		{"Модели Qwen разрабатывает компания Baidu.", "refuted"},
		{"Германия вышла из Евросоюза в октябре 2026 года.", "not_found or refuted"},
		{"Anthropic купила компанию Mistral AI в октябре 2026 года.", "not_found or refuted"},
		{"Роман «Война и мир» написал Лев Толстой.", "confirmed"},
		{"The Berlin Wall fell in 1979.", "refuted"},
	}

	for _, c := range claims {
		started := time.Now()
		result, err := CheckFact(c.claim)
		if err != nil {
			t.Logf("%-60.60s ERROR %v", c.claim, err)
			continue
		}
		t.Logf("%-60.60s %-11s want %-20s %4.1fs evidence %d", c.claim, result.Verdict, c.expected, time.Since(started).Seconds(), len(result.Evidence))
		for _, item := range result.Evidence {
			t.Logf("    supports=%v %.90s | %s", item.Supports, item.Quote, item.URL)
		}
	}
}
