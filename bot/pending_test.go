package bot

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

// Расписка должна собрать всё, что нужно для доставки ЧАСАМИ позже, когда ни
// обработчика, ни даже процесса уже нет.
func TestPendingNoteCollectsWhereToDeliver(t *testing.T) {
	message := &models.Message{
		ID:   77,
		Chat: models.Chat{ID: -100500},
		From: &models.User{ID: 42},
	}
	wait := newWaitMessage(31, waitText, okText, func(string) error { return nil })

	note := newPendingNote("video", message, wait, &aiCharge{userID: 42, paid: 2})

	switch {
	case note.row.Kind != "video":
		t.Errorf("вид %q", note.row.Kind)
	case note.row.ChatID != -100500:
		t.Errorf("чат %d", note.row.ChatID)
	case note.row.ReplyToID != 77:
		t.Errorf("сообщение-вопрос %d", note.row.ReplyToID)
	case note.row.WaitID != 31:
		t.Errorf("ждущее сообщение %d", note.row.WaitID)
	case note.row.UserID != 42:
		t.Errorf("пользователь %d", note.row.UserID)
	case note.row.PaidUnits != 2:
		t.Errorf("платных единиц %d, ожидалось 2", note.row.PaidUnits)
	}
}

// Пост от имени канала приходит без From, а ждущее сообщение могло не
// отправиться вовсе. Ни то, ни другое не должно ронять обработчик.
func TestPendingNoteSurvivesMissingSenderAndWait(t *testing.T) {
	message := &models.Message{ID: 5, Chat: models.Chat{ID: 7}}

	note := newPendingNote("image", message, nil, nil)

	if note.row.UserID != 0 {
		t.Errorf("пользователь %d, ожидался ноль", note.row.UserID)
	}
	if note.row.WaitID != 0 {
		t.Errorf("ждущее сообщение %d, ожидался ноль", note.row.WaitID)
	}
	if note.row.PaidUnits != 0 {
		t.Errorf("платных единиц %d, ожидался ноль", note.row.PaidUnits)
	}
}

// Расписка открывается ДО того, как сервис назовёт задачу, поэтому имя у неё
// есть всегда — своё, с меткой. Без этого смерть процесса между списанием и
// отправкой на сервис теряла деньги без следа.
func TestPendingNoteIsNamedBeforeTheServiceNamesIt(t *testing.T) {
	message := &models.Message{ID: 1, Chat: models.Chat{ID: 7}, From: &models.User{ID: 42}}

	note := newPendingNote("video", message, nil, nil)

	if note.row.JobID == "" {
		t.Fatal("расписка без имени — её нечем будет ни найти, ни снять")
	}
	if !isUnnamed(note.row.JobID) {
		t.Errorf("имя %q не помечено как ещё не данное сервисом", note.row.JobID)
	}
	if note.id != note.row.JobID {
		t.Error("расписка и строка разошлись в имени")
	}

	// Два запроса подряд не должны получить одно имя: иначе второй перетрёт
	// первый и один из людей останется без результата и без возврата.
	other := newPendingNote("video", message, nil, nil)
	if other.row.JobID == note.row.JobID {
		t.Error("две задачи получили одно имя")
	}
}

// Настоящее имя должно вытеснять временное, а не добавляться рядом.
func TestIsUnnamedTellsTheNamesApart(t *testing.T) {
	if !isUnnamed(unnamedJob(42)) {
		t.Error("своё имя не распознано как временное")
	}
	if isUnnamed("faba1ce1-243f-431b-be71-ff7fc135c782") {
		t.Error("имя от сервиса принято за временное — такую задачу не подберут")
	}
}

// Инлайн-задача: ни чата, ни сообщения-вопроса, зато есть инлайн-сообщение и
// подпись. Денег на этом пути нет вовсе, возвращать нечего.
func TestInlinePendingNoteKeepsWhereToEdit(t *testing.T) {
	note := newInlinePendingNote("video", "inline-77", 42, "кот в сапогах")

	switch {
	case note.row.InlineMessageID != "inline-77":
		t.Errorf("инлайн-сообщение %q", note.row.InlineMessageID)
	case note.row.Caption != "кот в сапогах":
		t.Errorf("подпись %q", note.row.Caption)
	case note.row.ChatID != 0:
		t.Errorf("чат %d, а Telegram его не называет вовсе", note.row.ChatID)
	case note.row.UserID != 42:
		t.Errorf("пользователь %d", note.row.UserID)
	case note.row.PaidUnits != 0:
		t.Errorf("платных единиц %d, а инлайн ничего не списывает", note.row.PaidUnits)
	case !isUnnamed(note.row.JobID):
		t.Error("инлайн-расписка открылась без метки")
	}
}

// Деньги без строки списания — ноль, а не паника: у человека без лимита
// aiCharge вообще не создаётся.
func TestPaidUnitsToleratesNoCharge(t *testing.T) {
	var none *aiCharge
	if got := none.paidUnits(); got != 0 {
		t.Errorf("без списания дало %d", got)
	}

	if got := (&aiCharge{free: 1, paid: 3}).paidUnits(); got != 3 {
		t.Errorf("дало %d, ожидалось 3", got)
	}
}

// Вид задачи решает, чем отдавать результат. Ошибка здесь означала бы попытку
// отправить mp4 как фотографию, то есть потерю готовой работы.
func TestResumedMediaMatchesTheKind(t *testing.T) {
	data := []byte("результат")

	for _, tc := range []struct {
		kind string
		want string
	}{
		{"image", "attach://image.png"},
		{"edit", "attach://image.png"},
		{"video", "attach://image.mp4"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			media, err := resumedMedia(tc.kind, data)
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}

			var got, caption string
			switch item := media.(type) {
			case *models.InputMediaPhoto:
				got, caption = item.Media, item.Caption
			case *models.InputMediaVideo:
				got, caption = item.Media, item.Caption
			default:
				t.Fatalf("неизвестный тип %T", media)
			}

			if got != tc.want {
				t.Errorf("вложение %q, ожидалось %q", got, tc.want)
			}
			if caption != recoveredCaption {
				t.Error("подобранный результат приходит без объяснения задержки")
			}
		})
	}

	if _, err := resumedMedia("перевод", data); err == nil {
		t.Error("неизвестный вид прошёл молча — такую строку никто не заметит")
	}
}

// Каждая попытка отправки обязана получить СВОЙ читатель: первая вычитывает
// его до конца, и повтор с тем же отправил бы пустоту. Грабля общая с deliver,
// проверяем её и здесь, потому что тут читатель создаётся в другом месте.
func TestResumedMediaGivesAFreshReaderEachTime(t *testing.T) {
	data := []byte("двадцать мегабайт, допустим")

	first, err := resumedMedia("video", data)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	second, err := resumedMedia("video", data)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	one := first.(*models.InputMediaVideo).MediaAttachment
	two := second.(*models.InputMediaVideo).MediaAttachment
	if one == two {
		t.Fatal("читатель один и тот же — повтор отправит пустоту")
	}

	buf := make([]byte, len(data))
	if n, _ := one.Read(buf); n != len(data) {
		t.Errorf("первый читатель отдал %d байт из %d", n, len(data))
	}
	if n, _ := two.Read(buf); n != len(data) {
		t.Errorf("второй читатель отдал %d байт из %d", n, len(data))
	}
}

// Между вопросом и подобранным ответом проходят часы, и вопрос могли удалить.
// Без этого флага Telegram отказал бы в отправке целиком — то есть пропавшее
// сообщение стоило бы человеку готового видео.
func TestResumeReplyAllowsAMissingQuestion(t *testing.T) {
	reply := resumeReply(77)
	if reply == nil {
		t.Fatal("ответа на вопрос нет вовсе")
	}
	if reply.MessageID != 77 {
		t.Errorf("отвечаем на %d", reply.MessageID)
	}
	if !reply.AllowSendingWithoutReply {
		t.Error("пропавший вопрос отменит отправку результата")
	}

	if resumeReply(0) != nil {
		t.Error("без вопроса параметры ответа не нужны")
	}
}
