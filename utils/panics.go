package utils

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"
)

// Паника — последняя дыра в маске секретов.
//
// Маска стоит на выходе стандартного логгера (см. secrets.go), но панику
// печатает не логгер, а рантайм — прямо в stderr, мимо log.SetOutput. А в
// ошибке go-telegram/bot лежит полный URL, то есть токен. Цена известна:
// 29.10.2026 `panic(err)` на старте бота уложил токен в журнал VPS 1292 раза,
// пока рядом обычные строки шли с маской.
//
// ExitOnPanic ловит панику, печатает её ЧЕРЕЗ МАСКУ и завершает процесс ровно
// так же, как завершила бы она сама. Поведение то же — systemd поднимет
// заново, — а в журнале чисто.
//
// Ставить нужно в КАЖДУЮ горутину отдельно: recover чужую панику не видит, и
// одна необёрнутая горутина сама напечатает токен. Обработчики апдейтов — это
// отдельная история, у них своя защита (recoverPanics в bot/bot.go), и она не
// завершает процесс: одна плохая картинка не должна рубить чужую генерацию.
// Горутины, запущенные ИЗ обработчика, тем recover'ом не прикрыты — его видно
// только в той горутине, где он объявлен.
func ExitOnPanic(what string) {
	problem := recover()
	if problem == nil {
		return
	}

	log.Print(panicLine(what, problem, debug.Stack()))
	os.Exit(1)
}

// panicLine makes the line a panic is reported with. Separate from ExitOnPanic
// so that a test can read it: os.Exit would take the test process with it.
//
// Маскируем здесь, а не только в писателе журнала, нарочно: паника может
// случиться и до HideSecretsInLogs — например, при чтении .env, где токен и
// лежит.
func panicLine(what string, problem any, stack []byte) string {
	return MaskSecrets(fmt.Sprintf("[error] panic in %s: %v\n%s", what, problem, stack))
}

// Go starts fn in a goroutine of its own, under the mask. Use it instead of a
// bare `go fn()` everywhere — see ExitOnPanic for why every goroutine needs
// its own.
func Go(what string, fn func()) {
	go func() {
		defer ExitOnPanic(what)

		fn()
	}()
}
