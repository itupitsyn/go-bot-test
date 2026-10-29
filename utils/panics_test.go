package utils

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// Та самая паника с 29.10.2026, дословно: библиотека кладёт в ошибку полный
// URL, а в URL Telegram токен и есть путь. Проверяем не ExitOnPanic целиком —
// os.Exit унёс бы с собой и тест, — а строку, которую он печатает.
func TestPanicLineHidesTheToken(t *testing.T) {
	problem := errors.New(`error call getMe, error do request for method getMe, ` +
		`Post "https://tg-api.example/bot1234567890:AAHfake-token-for-tests-only/getMe": ` +
		`context deadline exceeded`)

	got := panicLine("bot.New", problem, []byte("goroutine 1 [running]:\nmain.main()\n"))

	if strings.Contains(got, "AAHfake-token-for-tests-only") {
		t.Errorf("токен доехал до журнала: %s", got)
	}
	// Всё остальное обязано остаться: без имени метода, причины и стека строка
	// о панике бесполезна, а значит обёртку снимут.
	for _, needed := range []string{"bot.New", "getMe", "context deadline exceeded",
		"goroutine 1 [running]:"} {
		if !strings.Contains(got, needed) {
			t.Errorf("пропало %q: %s", needed, got)
		}
	}
}

// Маска не должна зависеть от того, успел ли кто-то позвать HideSecretsInLogs:
// паника возможна и до неё, например при чтении .env.
func TestPanicLineMasksWithoutTheLogWrapper(t *testing.T) {
	t.Setenv("TELEGRAM_APITOKEN", "1234567890:AAHfake-token-for-tests-only")

	got := panicLine("loadEnv", "токен не разобрать: 1234567890:AAHfake-token-for-tests-only", nil)

	if strings.Contains(got, "AAHfake-token-for-tests-only") {
		t.Errorf("токен остался: %s", got)
	}
}

func TestGoRunsTheWork(t *testing.T) {
	var once sync.WaitGroup
	once.Add(1)

	done := make(chan struct{})
	Go("test", func() {
		defer once.Done()
		close(done)
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Go не запустил работу")
	}

	once.Wait()
}

// ExitOnPanic обязан быть прозрачным, когда паники не было: он стоит в defer
// у каждой горутины, и лишнего шума от него быть не должно.
func TestExitOnPanicIsQuietWithoutAPanic(t *testing.T) {
	finished := false

	func() {
		defer ExitOnPanic("test")

		finished = true
	}()

	if !finished {
		t.Error("ExitOnPanic помешал обычному возврату")
	}
}
