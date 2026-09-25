package aiApi

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/joho/godotenv"
)

// A probe for tuning a single style: it draws one subject with several template
// variants so there is something to compare by eye.
//
//	AI_STYLE_PROBE=1 go test ./aiApi/ -run TestStyleCandidatesProbe -v -timeout 60m
//
// The variables are the same as for TestImageTemplateProbe: AI_IMAGE_PROBE_DIR
// and AI_IMAGE_PROBE_SEEDS. The service still picks the seed at random, so the
// variants are compared as samples, not pairwise.

// styleCandidates is what is being tuned right now. The table is live: it is
// edited for the task at hand and keeps no history.
var styleCandidates = []struct {
	slug     string
	subject  string            // Russian text without the keyword
	variants map[string]string // variant name -> template
}{
	// The cyberpunk template came out long, about fifty words. On a short
	// request like "нарисуй кота киберпанк" there is a risk that the style
	// overwhelms the subject itself and instead of a cat you get just a neon
	// street.
	{
		slug:     "cyber-cat",
		subject:  "кот",
		variants: map[string]string{"current": getImageTemplate("x киберпанк")},
	},
	{
		slug:     "cyber-portrait",
		subject:  "девушка с зонтом",
		variants: map[string]string{"current": getImageTemplate("x киберпанк")},
	},
}

func TestStyleCandidatesProbe(t *testing.T) {
	if os.Getenv("AI_STYLE_PROBE") == "" {
		t.Skip("пробник выключен; запуск: AI_STYLE_PROBE=1 go test ./aiApi/ -run TestStyleCandidatesProbe -v -timeout 60m")
	}

	if os.Getenv("AI_PAINTER_HOST") == "" {
		_ = godotenv.Load("../.env.local")
	}
	if os.Getenv("AI_PAINTER_HOST") == "" || os.Getenv("AI_LLM_URL") == "" {
		t.Skip("AI_PAINTER_HOST или AI_LLM_URL не заданы — пробнику некуда ходить")
	}

	dir := probeDir(t)
	seeds := probeSeeds()
	t.Logf("картинки лягут в %s (по %d на вариант)", dir, seeds)

	for _, c := range styleCandidates {
		// Translate once for all variants: the only difference should be in the
		// template.
		subject, err := translatePrompt(c.subject)
		if err != nil {
			t.Errorf("%s: перевод не удался: %v", c.slug, err)
			continue
		}

		names := make([]string, 0, len(c.variants))
		for name := range c.variants {
			names = append(names, name)
		}
		sort.Strings(names) // map iteration order is random, but a stable log is nicer

		for _, name := range names {
			prompt := applyPromptTemplate(c.variants[name], subject)
			t.Logf("[%s/%s] %s", c.slug, name, prompt)

			for i := 1; i <= seeds; i++ {
				imageBytes, err := requestImage(prompt, Caller{}, promptOrigin{})
				if err != nil {
					t.Errorf("%s/%s #%d: %v", c.slug, name, i, err)
					continue
				}

				file := fmt.Sprintf("%s_%s_%d.png", c.slug, name, i)
				if err := os.WriteFile(filepath.Join(dir, file), imageBytes, 0o644); err != nil {
					t.Errorf("не удалось записать %s: %v", file, err)
					continue
				}
				t.Logf("    -> %s (%d КБ)", file, len(imageBytes)/1024)
			}
		}
	}
}
