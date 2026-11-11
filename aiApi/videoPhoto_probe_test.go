package aiApi

import (
	"io"
	"os"
	"testing"

	"github.com/joho/godotenv"
)

// A probe, not a test: it shows a real photograph to the live llm and prints
// what the plan makes of the same request with the picture and without it.
// It asserts almost nothing; look by eye. Run:
//
//	AI_VIDEOPHOTO_PROBE=/path/to.png go test ./aiApi/ -run TestVideoPhotoProbe -v -timeout 20m
//
// The question it answers: does seeing the picture change the plan, and does
// it change it for the better. In the journal 101 video jobs of 164 are i2v,
// and until 11.11.2026 the plan never saw any of them.
func TestVideoPhotoProbe(t *testing.T) {
	path := os.Getenv("AI_VIDEOPHOTO_PROBE")
	if path == "" {
		t.Skip("AI_VIDEOPHOTO_PROBE не задан (в нём путь к картинке)")
	}
	if os.Getenv("AI_LLM_URL") == "" {
		_ = godotenv.Load("../.env.local")
	}
	if os.Getenv("AI_LLM_URL") == "" {
		t.Skip("AI_LLM_URL не задан — пробнику некуда ходить")
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("картинка не открылась: %v", err)
	}
	defer file.Close()

	image, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("картинка не прочиталась: %v", err)
	}

	photo := describeVideoPhoto(image)
	t.Logf("на фотографии: %s", photo)
	if photo == "" {
		t.Fatal("описания не вышло — дальше смотреть нечего")
	}

	// Те самые просьбы, на которых форма промахивалась стабильно: голая фраза
	// без кавычек и без глагола речи.
	for _, request := range []string{
		"верим Альхамдулиллах",
		"анимируй «привет, это я»",
		"пусть помашет рукой",
	} {
		for _, with := range []bool{false, true} {
			seen := ""
			if with {
				seen = photo
			}

			plan, err := planVideo(request, 5, seen)
			if err != nil {
				t.Errorf("%q (фото=%v): %v", request, with, err)
				continue
			}

			t.Logf("%-28s фото=%-5v форма=%-8s реплик=%d", request, with,
				plan.Form, len(plan.Lines))
			t.Logf("      %s", plan.Scene)
		}
	}
}
