package aiApi

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/joho/godotenv"
)

// Пробник для подкрутки одного стиля: рисует один сюжет несколькими
// вариантами шаблона, чтобы было что сравнить глазами.
//
//	AI_STYLE_PROBE=1 go test ./aiApi/ -run TestStyleCandidatesProbe -v -timeout 60m
//
// Переменные те же, что у TestImageTemplateProbe: AI_IMAGE_PROBE_DIR и
// AI_IMAGE_PROBE_SEEDS. Сид сервис по-прежнему берёт случайным, так что
// варианты сравниваются как выборки, а не попарно.

// styleCandidates — что именно сейчас крутим. Таблица живая: правится под
// текущую задачу, не хранит историю.
var styleCandidates = []struct {
	slug     string
	subject  string            // русский текст без ключевого слова
	variants map[string]string // имя варианта -> шаблон
}{
	// Киберпанковый шаблон вышел длинным, слов на пятьдесят. На коротком
	// запросе вроде «нарисуй кота киберпанк» есть риск, что стиль задавит сам
	// субъект и вместо кота выйдет просто неоновая улица.
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
		// Переводим один раз на все варианты — разница должна быть только
		// в шаблоне.
		subject, err := translatePrompt(c.subject)
		if err != nil {
			t.Errorf("%s: перевод не удался: %v", c.slug, err)
			continue
		}

		names := make([]string, 0, len(c.variants))
		for name := range c.variants {
			names = append(names, name)
		}
		sort.Strings(names) // порядок обхода map случайный, а лог хочется стабильный

		for _, name := range names {
			prompt := applyPromptTemplate(c.variants[name], subject)
			t.Logf("[%s/%s] %s", c.slug, name, prompt)

			for i := 1; i <= seeds; i++ {
				imageBytes, err := requestImage(prompt, Caller{})
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
