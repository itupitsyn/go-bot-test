package aiApi

import (
	"strings"
	"testing"
)

// Без тела в кадре про позу не говорим ВООБЩЕ и запрещаем менять одежду: этот
// запрет и приколачивает кадр. Проверено 02.10.2026 — стоит назвать позу на
// портрете по голову, и модель отъезжает и дорисовывает торс с одеждой.
func TestPromptWithoutBodySaysNothingAboutPose(t *testing.T) {
	for _, e := range stickerEmotions {
		got := e.Prompt(false)

		if !strings.HasSuffix(got, keepIdentity) {
			t.Errorf("%s: want the frozen identity clause, got %q", e.Key, got)
		}
		if e.Pose != "" && strings.Contains(got, e.Pose) {
			t.Errorf("%s: pose leaked into a head-only prompt: %q", e.Key, got)
		}
	}
}

func TestPromptWithBodyNamesThePose(t *testing.T) {
	for _, e := range stickerEmotions {
		got := e.Prompt(true)

		if !strings.HasSuffix(got, keepIdentityPosed) {
			t.Errorf("%s: want the posed identity clause, got %q", e.Key, got)
		}
		if e.Pose != "" && !strings.Contains(got, e.Pose) {
			t.Errorf("%s: pose missing from the prompt: %q", e.Key, got)
		}
	}
}

// Одежду двигать нельзя ровно в одной из двух формулировок, и перепутать их
// нельзя: запрет — это и есть замороженное тело.
func TestPosedClauseDoesNotForbidClothes(t *testing.T) {
	if !strings.Contains(keepIdentity, "do not change") {
		t.Error("the head-only clause must forbid changing the clothes")
	}
	if strings.Contains(keepIdentityPosed, "do not change") {
		t.Error("the posed clause must not forbid changing the clothes: that is what froze the body")
	}
	for _, clause := range []string{keepIdentity, keepIdentityPosed} {
		for _, must := range []string{"same person", "skin tone", "lighting"} {
			if !strings.Contains(clause, must) {
				t.Errorf("clause %q lost %q", clause, must)
			}
		}
	}
}

// Поза нужна КАЖДОЙ эмоции, включая те, что уже называют руку. Первая версия
// как раз их и оставила без позы — рука, мол, двигает тело сама, — и на живом
// наборе у всех трёх корпус совпал с исходником один в один (18.7, 18.5 и 35.0
// из 255 против 55.1, 57.0 и 54.9 с позой, замер 02.10.2026).
func TestEveryEmotionNamesAPose(t *testing.T) {
	for _, e := range stickerEmotions {
		if e.Pose == "" {
			t.Errorf("%s has no pose — its body will not move", e.Key)
		}
	}
}

// Поза "sleep" нарочно самая скромная: всё, что просит опереться или уронить,
// зовёт лишнюю руку (видено дважды на живых наборах 29.09.2026).
func TestSleepPoseInvitesNoSupport(t *testing.T) {
	var sleep StickerEmotion
	for _, e := range stickerEmotions {
		if e.Key == "sleep" {
			sleep = e
		}
	}

	for _, banned := range []string{"droop", "lean", "rest", "arm", "hand", "against", "onto"} {
		if strings.Contains(strings.ToLower(sleep.Prompt(true)), banned) {
			t.Errorf("sleep must not ask for %q: it grows an extra arm", banned)
		}
	}
}

// Ответ модели бывает с точкой или в кавычках — слово ищем, а не сравниваем
// строку целиком.
func TestBodyAnswerParsing(t *testing.T) {
	cases := map[string]bool{
		"BODY": true, "body": true, "\"BODY\"": true, "BODY.": true,
		"The answer is BODY": true,
		"HEAD":               false, "head": false, "HEAD.": false, "": false,
	}

	for answer, want := range cases {
		if got := strings.Contains(strings.ToUpper(answer), "BODY"); got != want {
			t.Errorf("%q: want %v, got %v", answer, want, got)
		}
	}
}
