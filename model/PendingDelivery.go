package model

import (
	"time"

	"gorm.io/gorm/clause"
)

// PendingDelivery — задача, которую сервис взял, а бот ещё не отдал человеку.
//
// ЗАЧЕМ ОНА ЕСТЬ. Генерация живёт не в боте. Сервис досчитывает её до конца,
// даже если бот умер, и держит готовое шесть часов, не удаляя при чтении.
// Значит, смерть бота посреди ожидания теряет не работу, а ТОЛЬКО ДОСТАВКУ —
// и доставку можно повторить, если помнить три вещи: как задача называется,
// куда отдавать и сколько с человека списано.
//
// Это второй слой защиты, и он нужен именно потому, что первый неполон.
// Плановый перезапуск бот пережидает сам, но ждёт ограниченное время, а задача
// вместе с очередью живёт дольше: по замерам за сутки средний i2v — 275
// секунд, средний t2v — 354, максимальное ожидание в очереди — 1025. Падение,
// OOM и снятие питания не пережидает вовсе ничто.
//
// Строка живёт недолго: появляется, когда сервис назвал задачу, и исчезает,
// когда та отдана или потеряна окончательно.
type PendingDelivery struct {
	// JobID — имя задачи на сервисе, оно же ключ. Одна задача отдаётся один
	// раз, и повторная запись того же имени не должна плодить строки.
	JobID string `gorm:"primaryKey"`
	// Kind — как генерация называет себя сама: image, edit или video. По нему
	// подбор знает, какой сервис спрашивать и чем отдавать результат.
	Kind string `gorm:"not null"`
	// ChatID, ReplyToID, WaitID — куда отдавать: чат, сообщение-вопрос и
	// ждущее сообщение, которое надо убрать после выдачи. Ноль во всех трёх
	// означает инлайн-задачу: у неё места доставки в этом смысле нет.
	ChatID    int64
	ReplyToID int
	WaitID    int
	// InlineMessageID — сообщение, вставленное инлайн-режимом в чужой чат.
	// Непустое значение означает, что отдавать надо правкой этого сообщения, а
	// не отправкой в чат: чат Telegram нам не называет вовсе. Идентификатор
	// остаётся годным и через часы, поэтому подобрать такую задачу можно.
	InlineMessageID string
	// Caption — исходный запрос, подпись к инлайн-результату. Хранится только
	// ради неё: в чужом чате под картинкой должно быть видно, что просили.
	// Живёт столько же, сколько строка, то есть минуты.
	Caption string
	// UserID и PaidUnits — чтобы вернуть деньги, когда задача потеряна
	// окончательно. Платные единицы хранятся числом, а не ссылкой на строку
	// списания: возврат идёт через AddAiCredit, которому нужен только счёт.
	//
	// Бесплатная квота тут нарочно не хранится. Она сама растворяется вместе с
	// окном в неделю, и восстанавливать её строку ради этого незачем — тогда
	// как деньги лежат, пока их не потратят.
	UserID    int64
	PaidUnits int
	CreatedAt time.Time
}

// RememberDelivery writes down a job we owe someone.
//
// Запись перетирает прежнюю с тем же именем: имя задачи уникально на сервисе,
// так что совпадение означает повторную регистрацию той же работы, а не вторую
// работу.
func RememberDelivery(delivery PendingDelivery) error {
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = time.Now()
	}

	return db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&delivery).Error
}

// RenameDelivery gives a job the name the service chose for it. Reports whether
// there was a row to rename at all.
//
// Переименование, а не вставка новой строки: расписка уже существует — она
// пишется до отправки на сервис, пока имени ещё нет, — и вторая строка о той
// же работе означала бы двойной возврат денег.
func RenameDelivery(from, to string) (bool, error) {
	res := db.Model(&PendingDelivery{}).
		Where("job_id = ?", from).
		Update("job_id", to)
	if res.Error != nil {
		return false, res.Error
	}

	return res.RowsAffected > 0, nil
}

// ForgetDelivery removes the record: the work is handed over, or lost for good.
func ForgetDelivery(jobID string) error {
	return db.Where("job_id = ?", jobID).Delete(&PendingDelivery{}).Error
}

// OwedDeliveries returns everything the bot still owes, oldest first.
//
// Без отсечки по времени нарочно: просроченную задачу тоже надо разобрать, а
// не забыть. Результата у неё уже нет, зато есть списанные деньги, и решает,
// что с ними делать, вызывающий.
func OwedDeliveries() ([]PendingDelivery, error) {
	var out []PendingDelivery
	err := db.Order("created_at").Find(&out).Error

	return out, err
}
