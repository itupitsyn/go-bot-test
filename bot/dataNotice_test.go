package bot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"telebot/model"
	"testing"
)

// Формулировка про данные опасна ровно одним: она называет конкретные числа, и
// если настройку поменяют, текст станет врать молча. Здесь проверяются те два
// числа, которые живут в этом же модуле. Остальные (6 часов, 7 суток срок
// вывода, 7 суток учёт) задаются в сервисе на питоне, и из Go их не видно —
// про это написано в шапке
// dataNotice.go, и при их изменении текст придётся править руками.
func TestDataNoticeMatchesInlineSettings(t *testing.T) {
	text := dataNoticeText()

	wantLimit := fmt.Sprintf("%d", model.InlineImageLimit)
	if !strings.Contains(text, wantLimit) {
		t.Errorf("в тексте нет размера буфера %s — InlineImageLimit поменяли, а формулировку нет", wantLimit)
	}

	wantDays := fmt.Sprintf("%d", int(model.InlineImageTTL.Hours()/24))
	if !strings.Contains(text, wantDays) {
		t.Errorf("в тексте нет срока %s суток — InlineImageTTL поменяли, а формулировку нет", wantDays)
	}
}

// Способы убрать данные надо называть именно теми командами, которые бот
// обрабатывает: обещание «очистить можно» без рабочей команды хуже молчания.
func TestDataNoticeNamesWorkingCommands(t *testing.T) {
	text := dataNoticeText()

	for _, command := range []string{"/forget", "/stickers_delete"} {
		if !strings.Contains(text, command) {
			t.Errorf("в тексте нет команды %s", command)
		}
	}
}

func TestDataNoticeIsNotEmpty(t *testing.T) {
	if strings.TrimSpace(dataNoticeText()) == "" {
		t.Fatal("текст про данные пустой")
	}
}

// Строка «соглашаешься с тем, что описано выше» имеет смысл только тогда, когда
// видно, что именно было описано. Привязка одна — дата редакции, поэтому она
// обязана быть в самом тексте, а не только в константе рядом.
func TestDataNoticeStatesConsentAndRevision(t *testing.T) {
	text := dataNoticeText()

	if !strings.Contains(text, "соглашаешься") {
		t.Error("в тексте нет строки про согласие")
	}

	if !strings.Contains(text, dataNoticeRevision) {
		t.Errorf("в тексте нет редакции %s — согласие не привязано к версии",
			dataNoticeRevision)
	}
}

// Отпечаток обязан быть настоящим хешем тела, а не однажды вписанной строкой.
// Вписанный руками он перестанет меняться вместе с текстом — и вся затея
// развалится молча, ровно так, как уже случилось с одной датой редакции.
func TestDataNoticeFingerprintMatchesBody(t *testing.T) {
	body := dataNoticeBody()
	sum := sha256.Sum256([]byte(body))
	want := hex.EncodeToString(sum[:3])

	text := dataNoticeText()
	if !strings.Contains(text, want) {
		t.Errorf("в тексте нет отпечатка %s, посчитанного от тела", want)
	}

	// И наоборот: отпечаток ДРУГОГО текста попасть туда не должен — иначе
	// проверка выше ничего не значит.
	other := sha256.Sum256([]byte(body + " "))
	if alien := hex.EncodeToString(other[:3]); alien != want &&
		strings.Contains(text, alien) {
		t.Errorf("в тексте оказался отпечаток %s не этого тела", alien)
	}
}

// Тело не должно нести строку редакции: иначе отпечаток считался бы от текста,
// в который он сам же и входит.
func TestDataNoticeBodyHasNoRevisionLine(t *testing.T) {
	if strings.Contains(dataNoticeBody(), "Редакция") {
		t.Error("строка редакции попала в тело — отпечаток станет замкнутым")
	}
}
