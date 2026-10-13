package bot

import (
	"context"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Самое важное свойство из всех: обработчик обязан получить РАБОЧИЙ контекст
// даже тогда, когда родительский уже отменён сигналом. Иначе бот честно
// досчитает двести секунд и не сможет отдать результат — все вызовы Telegram
// берут этот самый контекст.
func TestDetachedContextSurvivesCancelledParent(t *testing.T) {
	dead, cancel := context.WithCancel(context.Background())
	cancel()
	if dead.Err() == nil {
		t.Fatal("родительский контекст должен быть отменён — тест ни о чём")
	}

	// Проверять ОБЯЗАТЕЛЬНО внутри обработчика: снаружи уже отработал
	// defer cancel(), и контекст законно мёртв. На этом я и попался,
	// когда писал тест первым заходом.
	var called bool
	var errInside error
	var got context.Context
	handler := detachFromShutdown(func(ctx context.Context, _ *bot.Bot, _ *models.Update) {
		called, errInside, got = true, ctx.Err(), ctx
	})
	handler(dead, nil, nil)

	if !called {
		t.Fatal("обработчик не вызвался")
	}
	if errInside != nil {
		t.Errorf("обработчик получил мёртвый контекст: %v", errInside)
	}
	// И обратное: после возврата контекст обязан быть закрыт, иначе каждая
	// обработка оставляла бы за собой живой таймер.
	if got.Err() == nil {
		t.Error("после возврата контекст не закрыт — утечка")
	}
}

// «Переживает отмену» не должно означать «живёт вечно»: у отвязанного
// контекста обязан быть свой срок, иначе зависший обработчик держал бы
// перезапуск бесконечно.
func TestDetachedContextStillHasDeadline(t *testing.T) {
	t.Setenv("BOT_HANDLER_GRACE_SECONDS", "1")

	var deadline time.Time
	var ok bool
	handler := detachFromShutdown(func(ctx context.Context, _ *bot.Bot, _ *models.Update) {
		deadline, ok = ctx.Deadline()
	})
	handler(context.Background(), nil, nil)

	if !ok {
		t.Fatal("у отвязанного контекста нет срока — он бессмертен")
	}
	if left := time.Until(deadline); left <= 0 || left > 2*time.Second {
		t.Errorf("срок странный: осталось %v, ожидалось около секунды", left)
	}
}

// Значение контекста должно доезжать: WithoutCancel отвязывает отмену, но
// значения сохраняет, и на это опираются вызовы внутри.
func TestDetachedContextKeepsValues(t *testing.T) {
	type key struct{}
	parent := context.WithValue(context.Background(), key{}, "важное")

	var seen any
	handler := detachFromShutdown(func(ctx context.Context, _ *bot.Bot, _ *models.Update) {
		seen = ctx.Value(key{})
	})
	handler(parent, nil, nil)

	if seen != "важное" {
		t.Errorf("значение потерялось: %v", seen)
	}
}

func TestInFlightCountsAndReleases(t *testing.T) {
	if before := InFlight(); before != 0 {
		t.Fatalf("тест начался не с нуля: %d", before)
	}

	inside := int64(-1)
	handler := trackInFlight(func(_ context.Context, _ *bot.Bot, _ *models.Update) {
		inside = InFlight()
	})
	handler(context.Background(), nil, nil)

	if inside != 1 {
		t.Errorf("внутри обработчика счётчик %d, ожидалась единица", inside)
	}
	if after := InFlight(); after != 0 {
		t.Errorf("после обработчика счётчик %d, ожидался ноль", after)
	}
}

// Паника не должна оставлять счётчик завышенным: иначе завершение будет вечно
// сообщать о работе, которой нет, а оператор решит, что деплой завис.
func TestInFlightReleasesOnPanic(t *testing.T) {
	handler := trackInFlight(func(_ context.Context, _ *bot.Bot, _ *models.Update) {
		panic("нарочно")
	})

	func() {
		defer func() { _ = recover() }()
		handler(context.Background(), nil, nil)
	}()

	if after := InFlight(); after != 0 {
		t.Errorf("после паники счётчик %d, ожидался ноль", after)
	}
}

func TestHandlerGraceDefaultAndOverride(t *testing.T) {
	if got := HandlerGrace(); got != handlerGraceDefault {
		t.Errorf("без переменной дали %v, ожидалось %v", got, handlerGraceDefault)
	}

	t.Setenv("BOT_HANDLER_GRACE_SECONDS", "30")
	if got := HandlerGrace(); got != 30*time.Second {
		t.Errorf("с переменной дали %v, ожидалось 30s", got)
	}

	// Мусор в переменной не должен обнулять срок: ноль означал бы контекст,
	// просроченный в момент создания, то есть бота, который ничего не отдаёт.
	t.Setenv("BOT_HANDLER_GRACE_SECONDS", "ерунда")
	if got := HandlerGrace(); got != handlerGraceDefault {
		t.Errorf("при мусоре дали %v, ожидалось значение по умолчанию", got)
	}
	t.Setenv("BOT_HANDLER_GRACE_SECONDS", "0")
	if got := HandlerGrace(); got != handlerGraceDefault {
		t.Errorf("при нуле дали %v, ожидалось значение по умолчанию", got)
	}
}

func TestWatchShutdownReturnsOnSignal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		WatchShutdown(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("WatchShutdown не вернулся после отмены")
	}
}

// Ради этого всё и делалось: завершение обязано ДОЖДАТЬСЯ работающего
// обработчика. Проверка 13.10.2026 показала, что без своего ожидания процесс
// выходит мгновенно — библиотека уводит обработчик в отдельную горутину и
// Start его не ждёт.
func TestWaitForHandlersWaitsForRunningOne(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})

	handler := trackInFlight(func(_ context.Context, _ *bot.Bot, _ *models.Update) {
		close(started)
		<-release
	})
	go handler(context.Background(), nil, nil)
	<-started

	// Пока обработчик держится, ждать нас обязано и сдаться по сроку.
	left, ok := WaitForHandlers(300 * time.Millisecond)
	if ok {
		t.Error("вернулось «всё чисто», хотя обработчик ещё работает")
	}
	if left != 1 {
		t.Errorf("сообщило о %d работающих, ожидалась единица", left)
	}

	// А как отпустим — должно дождаться и сказать, что чисто.
	close(release)
	if left, ok := WaitForHandlers(2 * time.Second); !ok || left != 0 {
		t.Errorf("после завершения дало (%d, %v), ожидалось (0, true)", left, ok)
	}
}

func TestWaitForHandlersReturnsAtOnceWhenIdle(t *testing.T) {
	start := time.Now()
	left, ok := WaitForHandlers(5 * time.Second)
	if !ok || left != 0 {
		t.Errorf("на простое дало (%d, %v), ожидалось (0, true)", left, ok)
	}
	if spent := time.Since(start); spent > time.Second {
		t.Errorf("на простое потратило %v — должно возвращаться сразу", spent)
	}
}
