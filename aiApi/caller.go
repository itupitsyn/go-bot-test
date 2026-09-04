package aiApi

import (
	"errors"
	"strconv"
)

// ErrQueueFull — сервис отказался брать задачу: у этого человека их уже
// столько, сколько он может держать в работе одновременно. Это не поломка,
// а нормальный ответ, и наверху его надо отличать от «сервер подох».
var ErrQueueFull = errors.New("the user has too many jobs in flight")

// Caller — кто просит и куда сообщать о ходе дела.
//
// Ездит вместе через всю генерацию, потому что и то и другое нужно на всём её
// протяжении: по id сервис считает потолок задач и строит круг обслуживания,
// а колбэк ведёт сообщение ожидания.
type Caller struct {
	// UserID — id пользователя Telegram. Ноль означает «не знаем»: такие
	// задачи сервис относит к общему анонимному пользователю.
	UserID int64
	// Progress получает место задачи в очереди на каждом опросе; nil — не
	// отслеживать очередь.
	Progress ProgressFunc
}

// userJSON — владелец для json-тела запроса, вместе с ведущей запятой. Пустая
// строка, если владелец неизвестен: пусть сервис сам решает, что делать с
// безымянной задачей, а не получает наш ноль как настоящий id.
func (c Caller) userJSON() string {
	if c.UserID == 0 {
		return ""
	}

	return `, "user": ` + strconv.FormatInt(c.UserID, 10)
}

// userForm — владелец для multipart-запроса. Пустая строка — поле не слать.
func (c Caller) userForm() string {
	if c.UserID == 0 {
		return ""
	}

	return strconv.FormatInt(c.UserID, 10)
}
