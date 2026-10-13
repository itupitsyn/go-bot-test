package bot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/go-telegram/bot"
)

// shortPauses делает паузы незаметными: проверяем решения, а не умение ждать.
func shortPauses(t *testing.T) {
	t.Helper()

	was := deliveryPause
	deliveryPause = time.Millisecond
	t.Cleanup(func() { deliveryPause = was })
}

func TestDeliverDoesNotRetryWhatWorked(t *testing.T) {
	shortPauses(t)

	calls := 0
	got, err := deliver(context.Background(), "картинку", func() (string, error) {
		calls++

		return "готово", nil
	})

	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if got != "готово" {
		t.Errorf("вернулось %q", got)
	}
	if calls != 1 {
		t.Errorf("попыток %d, ожидалась одна", calls)
	}
}

// Главный случай: не доехало, но со второго раза ушло. Раньше здесь результат
// просто выбрасывали.
func TestDeliverRetriesWhatDidNotArrive(t *testing.T) {
	shortPauses(t)

	calls := 0
	got, err := deliver(context.Background(), "видео", func() (int, error) {
		calls++
		if calls < 3 {
			return 0, fmt.Errorf("error do request for method sendMediaGroup: %w",
				context.DeadlineExceeded)
		}

		return 42, nil
	})

	if err != nil {
		t.Fatalf("не отдало, хотя третья попытка удачная: %v", err)
	}
	if got != 42 {
		t.Errorf("вернулось %d", got)
	}
	if calls != 3 {
		t.Errorf("попыток %d, ожидалось три", calls)
	}
}

func TestDeliverGivesUpAfterLastAttempt(t *testing.T) {
	shortPauses(t)

	wanted := errors.New("прокси молчит")
	calls := 0
	_, err := deliver(context.Background(), "видео", func() (int, error) {
		calls++

		return 0, wanted
	})

	if !errors.Is(err, wanted) {
		t.Errorf("вернулась ошибка %v, ожидалась исходная", err)
	}
	if calls != deliveryAttempts {
		t.Errorf("попыток %d, ожидалось %d", calls, deliveryAttempts)
	}
}

// Отказ, который Telegram объяснил, повторять нельзя: он ответит то же самое,
// а человек будет ждать втрое дольше ни за что.
func TestDeliverDoesNotRetryRefusals(t *testing.T) {
	shortPauses(t)

	for _, tc := range []struct {
		name string
		err  error
	}{
		{"бот заблокирован", fmt.Errorf("%w, bot was blocked by the user", bot.ErrorForbidden)},
		{"нет чата", fmt.Errorf("%w, chat not found", bot.ErrorNotFound)},
		{"кривой запрос", fmt.Errorf("%w, message to edit not found", bot.ErrorBadRequest)},
		{"не тот токен", fmt.Errorf("%w, unauthorized", bot.ErrorUnauthorized)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, err := deliver(context.Background(), "видео", func() (int, error) {
				calls++

				return 0, tc.err
			})

			if err == nil {
				t.Fatal("ошибка потерялась")
			}
			if calls != 1 {
				t.Errorf("попыток %d, ожидалась одна", calls)
			}
		})
	}
}

// Пауза по просьбе Telegram, но не дольше потолка: иначе 429 с большим
// retry_after пересидел бы всю отсрочку на завершение.
func TestRetryPauseFollowsTelegram(t *testing.T) {
	shortPauses(t)

	asked := retryPause(&bot.TooManyRequestsError{RetryAfter: 7}, 1)
	if asked != 7*time.Second {
		t.Errorf("пауза %v, ожидалось 7s", asked)
	}

	capped := retryPause(&bot.TooManyRequestsError{RetryAfter: 600}, 1)
	if capped != maxRetryAfter {
		t.Errorf("пауза %v, ожидался потолок %v", capped, maxRetryAfter)
	}

	// Без просьбы — своя пауза, и перед третьей попыткой она длиннее.
	plain := errors.New("разрыв")
	if first, second := retryPause(plain, 1), retryPause(plain, 2); second <= first {
		t.Errorf("паузы не растут: %v, затем %v", first, second)
	}
}

// Когда ждать уже некогда (пришёл сигнал и вышла отсрочка), повторять нельзя,
// но и причину терять нельзя: именно она объясняет, что случилось.
func TestDeliverStopsWaitingOnDeadContext(t *testing.T) {
	shortPauses(t)

	dead, cancel := context.WithCancel(context.Background())
	cancel()

	wanted := errors.New("не доехало")
	calls := 0
	_, err := deliver(dead, "видео", func() (int, error) {
		calls++

		return 0, wanted
	})

	if !errors.Is(err, wanted) {
		t.Errorf("вернулось %v, ожидалась исходная причина", err)
	}
	if calls != 1 {
		t.Errorf("попыток %d, ожидалась одна: ждать было некогда", calls)
	}
}

// Грабля, из-за которой здесь функция, а не структура: читатель одноразовый.
// Этот тест показывает, что повтор получает ПОЛНОЕ тело, а не остаток.
func TestDeliverSendsWholeBodyOnRetry(t *testing.T) {
	shortPauses(t)

	body := []byte("двадцать мегабайт, допустим")
	var sent []int

	calls := 0
	_, err := deliver(context.Background(), "видео", func() (int, error) {
		// Так и на вызывающей стороне: читатель создаётся внутри замыкания.
		reader := bytes.NewReader(body)
		read, _ := io.ReadAll(reader)
		sent = append(sent, len(read))

		calls++
		if calls < 2 {
			return 0, errors.New("разрыв на полуслове")
		}

		return len(read), nil
	})

	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for attempt, size := range sent {
		if size != len(body) {
			t.Errorf("попытка %d отправила %d байт из %d", attempt+1, size, len(body))
		}
	}
}
