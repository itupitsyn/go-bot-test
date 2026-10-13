package bot

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/go-telegram/bot"
)

// Повтор доставки готового результата.
//
// ЗАЧЕМ. Генерация видео стоит двухсот секунд видеокарты, а иногда и звёзд
// человека. Отправка же делается ровно один раз: go-telegram/bot v1.15.0 в
// raw_request.go шлёт запрос и возвращает ошибку, никаких повторов внутри нет.
// 13.10.2026 прокси Telegram подтормаживал — sendMediaGroup не доехал, и
// досчитанное видео выбросили, хотя оно целиком лежало в памяти.
//
// Повторяем только ДОСТАВКУ, не генерацию: байты уже есть, вторая попытка
// ничего не стоит. На служебные сообщения («ждите», счёт очереди) это нарочно
// не распространяется — потерять строку текста не жалко, человек её даже не
// заметит, а вот обвешивать повторами каждый чих значит превратить короткий
// сбой в минутное залипание.
const (
	// deliveryAttempts — сколько всего попыток отдать результат.
	deliveryAttempts = 3
	// maxRetryAfter ограничивает паузу, которую назначает сам Telegram при
	// 429. Без потолка он мог бы попросить подождать дольше, чем нам вообще
	// отведено на завершение работы.
	maxRetryAfter = 30 * time.Second
)

// deliveryPause — пауза перед второй попыткой; перед третьей вдвое больше.
//
// Не доли секунды нарочно: лечим подтормаживающий прокси, а ему нужно время
// прийти в себя, а не три удара подряд. Переменная, а не константа, только
// ради тестов — им незачем ждать по-настоящему.
var deliveryPause = 3 * time.Second

// worthRetrying says whether another attempt has any chance at all.
//
// Библиотека оборачивает коды ответов в свои сентинелы (errors.go), поэтому
// гадать по тексту не нужно. Ответ получен и он отказ — повтор ничего не
// изменит: «chat not found» не найдётся, а человек, заблокировавший бота, не
// разблокирует его за три секунды. Зато всё, что НЕ ДОЕХАЛО — таймаут, разрыв,
// пятисотка от прокси — ровно тот случай, ради которого повтор и нужен.
func worthRetrying(err error) bool {
	switch {
	case errors.Is(err, bot.ErrorBadRequest),
		errors.Is(err, bot.ErrorForbidden),
		errors.Is(err, bot.ErrorUnauthorized),
		errors.Is(err, bot.ErrorNotFound):
		return false
	}

	return true
}

// retryPause is how long to wait before the next attempt.
//
// Если Telegram сам сказал, сколько ждать, слушаем его: своя пауза короче его
// просьбы приведёт только ко второму отказу.
func retryPause(err error, attempt int) time.Duration {
	var tooMany *bot.TooManyRequestsError
	if errors.As(err, &tooMany) && tooMany.RetryAfter > 0 {
		pause := time.Duration(tooMany.RetryAfter) * time.Second
		if pause > maxRetryAfter {
			return maxRetryAfter
		}

		return pause
	}

	return deliveryPause * time.Duration(attempt)
}

// deliver sends the finished work, trying again when the attempt did not reach
// Telegram. `what` goes into the log as is.
//
// ВАЖНО О ЗАМЫКАНИИ: send обязан каждый раз собирать параметры ЗАНОВО. Тело
// файла передаётся читателем (bytes.NewReader), а читатель одноразовый: первая
// попытка вычитывает его до конца, и повтор с тем же читателем отправил бы
// пустоту — то есть «успешно доставил» ничего. Поэтому здесь функция, а не
// готовая структура параметров.
func deliver[T any](ctx context.Context, what string, send func() (T, error)) (T, error) {
	var zero T

	for attempt := 1; ; attempt++ {
		res, err := send()
		if err == nil {
			if attempt > 1 {
				log.Printf("Отдал %s с %d-й попытки\n", what, attempt)
			}

			return res, nil
		}

		if attempt >= deliveryAttempts || !worthRetrying(err) {
			return zero, err
		}

		pause := retryPause(err, attempt)
		log.Printf("[warn] %s не ушло с попытки %d, повтор через %s: %v\n",
			what, attempt, pause, err)

		select {
		case <-ctx.Done():
			// Ждать стало некогда. Возвращаем ПЕРВОНАЧАЛЬНУЮ причину: по ней
			// потом и разбираться, а «context canceled» не объясняет ничего.
			return zero, err
		case <-time.After(pause):
		}
	}
}
