package aiApi

import (
	"os"
	"testing"
	"time"
)

// A probe, not a test: it goes to the live llm and the live search and prints
// what it decided. Run with the addresses and the key in the environment:
//
//	AI_FACTCHECK_PROBE=1 AI_LLM_URL=... FACTCHECK_SEARCH_URL=... BRAVE_SEARCH_KEY=... go test ./aiApi/ -run TestFactCheckProbe -v -timeout 30m
//
// With BRAVE_SEARCH_KEY set the probe uses Brave without counting the requests
// in the bot's database: about 20 of them, inside the margin kept below the
// monthly budget. It is here to see that the Go chain behaves like the probe it
// was measured with.
func TestFactCheckProbe(t *testing.T) {
	if os.Getenv("AI_FACTCHECK_PROBE") == "" {
		t.Skip("probe: set AI_FACTCHECK_PROBE=1")
	}
	saved := BraveQuota
	BraveQuota = func() bool { return true }
	t.Cleanup(func() { BraveQuota = saved })

	claims := []struct{ claim, expected string }{
		{"James Talarico defeated Ken Paxton in the 2026 Texas US Senate race.", "confirmed"},
		{"слышал, Пакстон выиграл выборы в Сенат в Техасе?", "refuted"},
		{"Чемпионат мира по футболу 2018 года проходил в Бразилии.", "refuted"},
		{"Роман «Война и мир» написал Лев Толстой.", "confirmed"},
		{"Anthropic купила компанию Mistral AI в октябре 2026 года.", "not_found or refuted"},
		{"у меня лапки", "not_checkable"},
		{"кто съел мой йогурт?", "not_checkable"},
		// Shaped like the bot builds a claim out of a caption and pictures.
		{"глянь, что пишут\n\nТекст на картинке:\nElon Musk @elonmusk · 3h\nTesla will open a factory on the Moon in 2027.", "not_found or refuted"},
		{"Текст на картинке 1:\nЧемпионат мира по футболу 2018 года прошёл в России\n\nТекст на картинке 2:\nВ финале сборная Франции обыграла Хорватию 4:2", "confirmed"},
	}

	var recent []string
	for _, c := range claims {
		started := time.Now()
		result, err := CheckFact(c.claim)
		if err != nil {
			t.Logf("%-55.55q ERROR %v", c.claim, err)
			continue
		}
		t.Logf("%-55.55q %-13s want %-20s %4.1fs evidence %d", c.claim, result.Verdict, c.expected, time.Since(started).Seconds(), len(result.Evidence))
		if result.Primary != nil {
			t.Logf("    primary: %s", result.Primary.URL)
		}
		for _, item := range result.Evidence {
			t.Logf("    supports=%v %.90s | %s", item.Supports, item.Quote, item.URL)
		}
		if result.Verdict == FactNotCheckable {
			joke, err := GenerateFactJoke(c.claim, recent)
			t.Logf("    joke: %q %v", joke, err)
			if joke != "" {
				recent = append(recent, joke)
			}
		}
	}
}
