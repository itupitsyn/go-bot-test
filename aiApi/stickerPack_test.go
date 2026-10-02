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
		got := e.Prompt(false, testScreen)

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
		got := e.Prompt(true, testScreen)

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
		if strings.Contains(strings.ToLower(sleep.Prompt(true, testScreen)), banned) {
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

// testScreen — цвет по умолчанию: с ним промт совпадает с тем, что уходило в
// сервис до выбора цвета под фотографию.
var testScreen = screenColours[0]

// Фон вырезается по цвету (см. cutout.py на стороне сервиса), поэтому просьба
// об экране обязана быть в КАЖДОМ промте — и с телом, и без него.
func TestEveryPromptAsksForTheScreen(t *testing.T) {
	for _, screen := range screenColours {
		for _, e := range stickerEmotions {
			for _, withBody := range []bool{true, false} {
				if !strings.Contains(e.Prompt(withBody, screen), screen.clause()) {
					t.Errorf("%s (тело=%v, экран=%s): нет просьбы про фон: %q",
						e.Key, withBody, screen.Word, e.Prompt(withBody, screen))
				}
			}
		}
	}
}

// Просьба про фон идёт ДО требования сохранить человека: так она проверена на
// живых генерациях 02.10.2026 (зелёного в кадре 50-91%).
func TestScreenComesBeforeTheIdentityClause(t *testing.T) {
	for _, e := range stickerEmotions {
		got := e.Prompt(true, testScreen)
		if strings.Index(got, testScreen.clause()) > strings.Index(got, keepIdentityPosed) {
			t.Errorf("%s: просьба про фон оказалась после требования о человеке", e.Key)
		}
	}
}

// Формулировка просьбы про зелёный экран не должна меняться молча: именно на
// ней замерено, что модель перестала растворять руку в фоне.
func TestGreenClauseKeepsTheMeasuredWording(t *testing.T) {
	want := ", replace the background behind them with a flat solid chroma key " +
		"green screen, the green must stay behind them and must not touch the person"

	if got := screenColours[0].clause(); got != want {
		t.Errorf("просьба про зелёный экран разошлась с замеренной:\n%q", got)
	}
}

// Обе руки названы — это лекарство от третьей. Без второй руки в промте модель
// вешала её на подставленное позой плечо.
func TestApproveAccountsForBothArms(t *testing.T) {
	var approve StickerEmotion
	for _, e := range stickerEmotions {
		if e.Key == "approve" {
			approve = e
		}
	}

	got := strings.ToLower(approve.Prompt(true, testScreen))
	for _, want := range []string{"with one hand", "the other arm"} {
		if !strings.Contains(got, want) {
			t.Errorf("approve: в промте нет %q: %q", want, got)
		}
	}
}

// Переделать один стикер можно, только если по нему удаётся понять, какая это
// эмоция. Узнаём по эмодзи — это всё, что Telegram рассказывает о стикере.
func TestStickerEmotionByEmoji(t *testing.T) {
	for _, e := range stickerEmotions {
		for _, emoji := range e.Emoji {
			got, ok := StickerEmotionByEmoji(emoji)
			if !ok {
				t.Errorf("%s: эмодзи %q не нашёлся", e.Key, emoji)
				continue
			}
			if got.Key != e.Key {
				t.Errorf("%s: эмодзи %q привёл к %s", e.Key, emoji, got.Key)
			}
		}
	}

	if _, ok := StickerEmotionByEmoji("\U0001F680"); ok {
		t.Error("чужой эмодзи не должен находиться")
	}
	if _, ok := StickerEmotionByEmoji(""); ok {
		t.Error("пустой эмодзи не должен находиться")
	}
}

// И это работает, только пока эмодзи у эмоций не повторяются: на общий эмодзи
// поиск вернул бы первую попавшуюся, и человек получил бы вместо сломанного
// стикера совсем другой.
func TestStickerEmojiAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, e := range stickerEmotions {
		for _, emoji := range e.Emoji {
			if other, busy := seen[emoji]; busy {
				t.Errorf("эмодзи %q есть и у %s, и у %s", emoji, other, e.Key)
			}
			seen[emoji] = e.Key
		}
	}
}

// Пустой эмодзи у эмоции сделал бы её непочинимой: ответ на её стикер было бы
// не с чем сопоставить.
func TestEveryEmotionHasAnEmoji(t *testing.T) {
	for _, e := range stickerEmotions {
		if len(e.Emoji) == 0 {
			t.Errorf("%s: нет ни одного эмодзи", e.Key)
		}
		for _, emoji := range e.Emoji {
			if emoji == "" {
				t.Errorf("%s: пустой эмодзи в списке", e.Key)
			}
		}
	}
}
