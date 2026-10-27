package bot

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

// Длину пишут и в конце, и сразу после команды, слитно и раздельно, по-русски
// и по-английски. Все эти написания — одна и та же просьба, и из промпта
// число должно уйти: промпт идёт в llm на перевод, и «10 секунд» в нём
// превратилось бы в часть сцены.
func TestVideoSecondsReadFromBothEnds(t *testing.T) {
	cases := []struct {
		text string
		want float64
		rest string
	}{
		{"котика 10 секунд", 10, "котика"},
		{"10 секунд котика", 10, "котика"},
		{"котика 10с", 10, "котика"},
		{"котика 10 с", 10, "котика"},
		{"котика 10 сек.", 10, "котика"},
		{"котика на 10 секунд,", 10, "котика на"},
		{"a cat 10 seconds", 10, "a cat"},
		{"a cat 10s", 10, "a cat"},
		{"10 секунд", 10, ""},
		// Камера дописывается в конце, и после выреза длины она там и
		// остаётся: режется только число с единицей, слова не трогаются.
		{"котика вокруг 10 секунд", 10, "котика вокруг"},
		{"a cat around 10 seconds", 10, "a cat around"},
	}

	for _, c := range cases {
		got, rest := cutVideoSeconds(c.text)
		if got != c.want || rest != c.rest {
			t.Errorf("cutVideoSeconds(%q) = %.0f, %q; ждали %.0f, %q",
				c.text, got, rest, c.want, c.rest)
		}
	}
}

// Длины не просили — текст возвращается ТОТ ЖЕ, байт в байт. По кавычкам и
// пробелам в нём собирается речь в видео, и переклеивать его из слов ради
// запроса, которого не было, незачем. Число посреди текста — часть сцены, а
// число без единицы — вообще не длина.
func TestNoLengthLeavesTheTextAlone(t *testing.T) {
	cases := []string{
		"котика",
		"котика, который  10 секунд ждёт у двери",
		"котика 10",
		"котика 0 секунд",
		"десять секунд котика",
		"котика медленно",
		// Длина не на краю — её не видно, и это осознанно: выйдет короткий
		// ролик по короткой цене, а не длинный втихую. Камера при этом
		// работает как работала, её читает llm по всему тексту.
		"котика 10 секунд вокруг",
		"",
	}

	for _, text := range cases {
		got, rest := cutVideoSeconds(text)
		if got != 0 || rest != text {
			t.Errorf("cutVideoSeconds(%q) = %.0f, %q; ждали 0 и текст без изменений",
				text, got, rest)
		}
	}
}

// В кавычках стоит реплика, и секунды в ней — слова, которые произнесут
// вслух. Вырезать их значило бы и укоротить реплику, и оставить кавычку без
// пары: речь в видео собирается именно по кавычкам.
func TestLengthInQuotesIsSpokenWords(t *testing.T) {
	cases := []string{
		`мужика «я подожду 10 секунд»`,
		`a man "wait 10 seconds"`,
		`«10 секунд» на табло`,
	}

	for _, text := range cases {
		got, rest := cutVideoSeconds(text)
		if got != 0 || rest != text {
			t.Errorf("cutVideoSeconds(%q) = %.0f, %q; реплику трогать нельзя",
				text, got, rest)
		}
	}
}

// Просьбу зажимаем в то, что реально поедет на карту, а не отказываем: сервис
// зажмёт её всё равно, а человек просил ролик. И считать потом надо по этому
// же числу — иначе попросят минуту, получат десять секунд, а в счёт попадёт
// минута.
func TestLengthIsClampedToWhatTheCardWillDo(t *testing.T) {
	cases := map[string]float64{
		"котика 60 секунд":     videoSecondsMax,
		"котика 100500 секунд": videoSecondsMax,
		"котика 16 секунд":     videoSecondsMax,
		// А вот это с 27.10.2026 уже внутри диапазона и доезжает как есть:
		// до замера пятнадцати секунд одиннадцать зажимались в десять.
		"котика 11 секунд": 11,
		"котика 3 секунды":     videoSecondsMin,
		"котика 1 с":           videoSecondsMin,
		"котика 5 секунд":      videoSecondsDefault,
	}

	for text, want := range cases {
		if got, _ := cutVideoSeconds(text); got != want {
			t.Errorf("cutVideoSeconds(%q) = %.0f, ждали %.0f", text, got, want)
		}
	}
}

// Длина — только из того, что человек набрал сам. В сообщении, на которое он
// ответил, «10 секунд» — чужие слова и часть сцены, и десять минут карты по
// ним заказывать нельзя.
func TestLengthComesOnlyFromTheTypedText(t *testing.T) {
	typed := &models.Message{
		Text:           "анимируй 10 секунд",
		ReplyToMessage: &models.Message{Text: "котик на подоконнике"},
	}
	if prompt, seconds := buildAnimationRequest(typed, "анимируй", "animate"); seconds != 10 ||
		prompt != "котик на подоконнике" {
		t.Errorf("своя просьба: %q, %.0f", prompt, seconds)
	}

	quoted := &models.Message{
		Text:           "анимируй",
		ReplyToMessage: &models.Message{Text: "он молчал 10 секунд"},
	}
	if prompt, seconds := buildAnimationRequest(quoted, "анимируй", "animate"); seconds != 0 ||
		prompt != "он молчал 10 секунд" {
		t.Errorf("чужая сцена: %q, %.0f", prompt, seconds)
	}
}
