package aiApi

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/joho/godotenv"
)

// Пробник, а не тест: рисует один и тот же сюжет старым и новым шаблоном и
// раскладывает PNG по папкам. Ничего не утверждает — сравнивать глазами.
//
//	AI_IMAGE_PROBE=1 go test ./aiApi/ -run TestImageTemplateProbe -v -timeout 60m
//
// Переменные: AI_IMAGE_PROBE_DIR — куда класть (по умолчанию ./probe_images в
// корне репозитория), AI_IMAGE_PROBE_SEEDS — сколько картинок на каждую пару
// сюжет+шаблон (по умолчанию 3).
//
// Чего пробник НЕ умеет: у /api/txt2img нет параметра сида, сервис берёт его
// случайным. Значит арки сравниваются не попарно, а как две выборки — смотреть
// надо на общее впечатление от пачки, а не на «вот эта против вот этой».

// legacyTemplates — то, что стояло до перехода на Z-Image: пресеты Fooocus под
// SDXL с CFG 5-7. Держим здесь только ради сравнения.
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
		// Тесты бегут из aiApi/, а класть удобнее рядом с проектом.
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
		slug  string // латиницей: попадает в имя файла
		style string // ключевое слово, оно же ключ в legacyTemplates
		text  string // что «написал пользователь», уже без команды
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
		// Ключевое слово в конце — так его напишет пользователь, и так его
		// увидит getImageTemplate.
		userText := c.text
		if c.style != "обычный" {
			userText = c.text + " " + c.style
		}

		// Переводим ОДИН раз и отдаём обеим аркам один и тот же текст: иначе
		// разброс перевода подмешается в разницу шаблонов.
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
