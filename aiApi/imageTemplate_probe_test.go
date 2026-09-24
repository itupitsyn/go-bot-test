package aiApi

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/joho/godotenv"
)

// A probe, not a test: it draws the same subject with the old and the new
// template and sorts the PNGs into folders. It asserts nothing; compare by eye.
//
//	AI_IMAGE_PROBE=1 go test ./aiApi/ -run TestImageTemplateProbe -v -timeout 60m
//
// Variables: AI_IMAGE_PROBE_DIR is where to put them (./probe_images in the
// repository root by default), AI_IMAGE_PROBE_SEEDS is how many images per
// subject+template pair (3 by default).
//
// What the probe CANNOT do: /api/txt2img has no seed parameter, the service
// picks it at random. So the arms are compared not pairwise but as two samples:
// look at the overall impression of the batch, not at "this one versus that
// one".

// legacyTemplates is what we had before switching to Z-Image: Fooocus presets
// for SDXL with CFG 5-7. Kept here only for comparison.
var legacyTemplates = map[string]string{
	"обычный":     "%s",
	"аниме":       "%s . anime style, key visual, vibrant, studio anime, highly detailed",
	"реалистично": "%s, RAW candid cinema, 16mm, color graded portra 400 film, remarkable color, ultra realistic, textured skin, remarkable detailed pupils, realistic dull skin noise, visible skin detail, skin fuzz, dry skin, shot with cinematic camera",
	"киберпанк":   "%s . neon, dystopian, futuristic, digital, vibrant, detailed, high contrast, reminiscent of cyberpunk genre video games",
	"меха":        "%s . it should look like it does in a real ife . blend of organic and mechanical elements, futuristic, cybernetic, detailed, intricate",
}

func probeSeeds() int {
	if raw := os.Getenv("AI_IMAGE_PROBE_SEEDS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return 3
}

func probeDir(t *testing.T) string {
	dir := os.Getenv("AI_IMAGE_PROBE_DIR")
	if dir == "" {
		// Tests run from aiApi/, but it is handier to put the output next to
		// the project.
		dir = filepath.Join("..", "probe_images")
	}

	absolute, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("не удалось разобрать путь %q: %v", dir, err)
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		t.Fatalf("не удалось создать %q: %v", absolute, err)
	}

	return absolute
}

func TestImageTemplateProbe(t *testing.T) {
	if os.Getenv("AI_IMAGE_PROBE") == "" {
		t.Skip("пробник выключен; запуск: AI_IMAGE_PROBE=1 go test ./aiApi/ -run TestImageTemplateProbe -v -timeout 60m")
	}

	if os.Getenv("AI_PAINTER_HOST") == "" {
		_ = godotenv.Load("../.env.local")
	}
	if os.Getenv("AI_PAINTER_HOST") == "" || os.Getenv("AI_LLM_URL") == "" {
		t.Skip("AI_PAINTER_HOST или AI_LLM_URL не заданы — пробнику некуда ходить")
	}

	cases := []struct {
		slug  string // latin letters: it goes into the file name
		style string // the keyword, also the key in legacyTemplates
		text  string // what "the user wrote", already without the command
	}{
		{"cat-plain", "обычный", "рыжий кот на подоконнике, за окном снег"},
		{"girl-anime", "аниме", "девушка с зонтом под дождём"},
		{"oldman-real", "реалистично", "пожилой рыбак чинит сеть на причале"},
		{"street-cyber", "киберпанк", "узкая улица с лапшичной и вывесками"},
		{"robot-meha", "меха", "робот сидит на камне и смотрит на закат"},
	}

	dir := probeDir(t)
	seeds := probeSeeds()
	t.Logf("картинки лягут в %s (по %d на шаблон)", dir, seeds)

	for _, c := range cases {
		// The keyword goes at the end: that is how the user writes it and how
		// getImageTemplate sees it.
		userText := c.text
		if c.style != "обычный" {
			userText = c.text + " " + c.style
		}

		// Translate ONCE and give both arms the same text: otherwise the spread
		// of the translation leaks into the difference between the templates.
		subject, err := translatePrompt(getImagePrompt(userText))
		if err != nil {
			t.Errorf("%s: перевод не удался: %v", c.slug, err)
			continue
		}

		arms := map[string]string{
			"old": legacyTemplates[c.style],
			"new": getImageTemplate(userText),
		}

		for arm, template := range arms {
			prompt := applyPromptTemplate(template, subject)
			t.Logf("[%s/%s] %s", c.slug, arm, prompt)

			for i := 1; i <= seeds; i++ {
				imageBytes, err := requestImage(prompt, Caller{})
				if err != nil {
					t.Errorf("%s/%s #%d: %v", c.slug, arm, i, err)
					continue
				}

				name := fmt.Sprintf("%s_%s_%d.png", c.slug, arm, i)
				if err := os.WriteFile(filepath.Join(dir, name), imageBytes, 0o644); err != nil {
					t.Errorf("не удалось записать %s: %v", name, err)
					continue
				}
				t.Logf("    -> %s (%d КБ)", name, len(imageBytes)/1024)
			}
		}
	}
}
