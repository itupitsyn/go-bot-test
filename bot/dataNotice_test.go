package bot

import (
	"fmt"
	"strings"
	"telebot/model"
	"testing"
)

// Формулировка про данные опасна ровно одним: она называет конкретные числа, и
// если настройку поменяют, текст станет врать молча. Здесь проверяются те два
// числа, которые живут в этом же модуле. Остальные (6 часов, 7 суток, 30 суток)
// задаются в сервисе на питоне, и из Go их не видно — про это написано в шапке
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
