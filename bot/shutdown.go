package bot

import (
	"context"
	"log"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Корректное завершение работы.
//
// БИБЛИОТЕКА НЕ ЖДЁТ. Я сначала решил обратное, прочитав waitUpdates: там
// b.ProcessUpdate вызывается синхронно, а Bot.Start ждёт свои горутины. Но
// внутри ProcessUpdate стоит `go r(ctx, b, upd)` — обработчик уходит в
// ОТДЕЛЬНУЮ горутину, рабочая возвращается сразу по ctx.Done(), Start выходит,
// main завершается и убивает обработчик вместе с чужой генерацией.
//
// Проверено вживую 13.10.2026 в 22:04 на работающей t2v: счётчик честно
// написал «waiting for 1 handler(s)», и в ту же секунду процесс вышел, а
// `systemctl restart` вернулся за ноль секунд. Поэтому ожидание здесь СВОЁ —
// WaitForHandlers ниже.
//
// ЧЕГО НЕ ХВАТАЛО, И ЭТО ЧЕТЫРЕ РАЗНЫЕ ВЕЩИ:
//
//  1. SIGTERM не был заведён в signal.NotifyContext — только os.Interrupt.
//     `docker stop` посылает именно SIGTERM, обработчика у него не было, и
//     процесс умирал мгновенно. Всё ожидание в библиотеке при этом ни разу не
//     получало шанса сработать.
//
//  2. Контекст, отменённый по сигналу, ломает ВЫДАЧУ. Все вызовы Telegram
//     берут тот же ctx, и после отмены SendVideo падает сразу. Бот честно
//     досчитал бы двести секунд и не смог отдать результат — хуже, чем
//     сейчас, потому что выглядит работающим. Отсюда detachFromShutdown.
//
//  3. Никто не дожидался обработчиков после возврата Start — см. выше.
//     Отсюда WaitForHandlers, который вызывается из main.
//
//  4. TimeoutStopSec у systemd-юнита. По умолчанию 90 секунд, потом SIGKILL,
//     а i2v считается около двухсот. Без этого первые три пункта — декорация.
//     Настройка живёт не в репозитории, а на сервере: дроп-ин
//     /etc/systemd/system/raffle-bot.service.d/stop-timeout.conf, 330 секунд.

// handlerGraceDefault — сколько даём обработчику дожить после сигнала.
//
// Пять минут: самое долгое, что бот ждёт синхронно, — это видео, около двухсот
// секунд, плюс запас на выдачу файла в Telegram. Не бесконечность нарочно:
// зависший обработчик не должен держать перезапуск вечно.
const handlerGraceDefault = 5 * time.Minute

// inFlight — сколько обработчиков прямо сейчас в работе.
//
// На нём держится и ожидание при завершении, и строка в логе. Строка нужна не
// меньше: `systemctl restart`, висящий три минуты, выглядит как зависший
// деплой, и его добьют руками — ровно в тот момент, когда всё работало
// правильно.
var inFlight atomic.Int64

// waitTick — как часто поглядывать на счётчик при завершении.
const waitTick = 200 * time.Millisecond

// HandlerGrace is how long a handler may keep going after the stop signal.
func HandlerGrace() time.Duration {
	if raw := os.Getenv("BOT_HANDLER_GRACE_SECONDS"); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
		log.Println("[error] BOT_HANDLER_GRACE_SECONDS не число, беру по умолчанию:", raw)
	}
	return handlerGraceDefault
}

// InFlight is how many handlers are running right now.
func InFlight() int64 {
	return inFlight.Load()
}

// trackInFlight counts running handlers so that shutdown can say what it waits
// for.
//
// Снимается через defer, поэтому паника внутри счётчик не ломает: recoverPanics
// стоит внутри этого middleware, но даже без него defer отработал бы.
func trackInFlight(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		inFlight.Add(1)
		defer inFlight.Add(-1)

		next(ctx, b, update)
	}
}

// detachFromShutdown gives the handler a context that survives the stop signal.
//
// context.WithoutCancel отвязывает от отмены, сохраняя значения; поверх него
// свой срок, чтобы «переживает отмену» не превратилось в «живёт вечно».
//
// Важно, что отвязан только контекст ОБРАБОТЧИКА. Контекст, по которому
// библиотека решает брать ли новые апдейты, остаётся прежним: новое перестаёт
// приходить сразу, а начатое доживает и успевает отдать результат.
func detachFromShutdown(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		detached, cancel := context.WithTimeout(
			context.WithoutCancel(ctx), HandlerGrace())
		defer cancel()

		next(detached, b, update)
	}
}

// WaitForHandlers blocks until no handler is running, or the limit passes.
// Returns how many are still going and whether we got to zero in time.
//
// Опросом счётчика, а не через sync.WaitGroup, и это не лень. Add у WaitGroup
// пришлось бы делать внутри middleware, то есть УЖЕ В ГОРУТИНЕ обработчика —
// а значит он мог бы не успеть до Wait, и Wait вернулся бы на нуле, бросив
// работу. У WaitGroup это прямо запрещено: «Add с положительной дельтой при
// нулевом счётчике должен случиться раньше Wait». Атомарный счётчик такого
// ограничения не имеет, а цена — заглядывать в него пять раз в секунду.
//
// Остаётся щель: апдейт, пришедший в то же мгновение, что и сигнал, может
// успеть создать горутину после того, как мы увидели нуль. Закрыть её изнутри
// нельзя — горутину создаёт библиотека, не мы. На практике это доли секунды
// против двухсот секунд генерации, и теряется в худшем случае только что
// начатый запрос, а не досчитанный.
func WaitForHandlers(limit time.Duration) (int64, bool) {
	deadline := time.Now().Add(limit)
	for {
		if left := InFlight(); left == 0 {
			return 0, true
		} else if !time.Now().Before(deadline) {
			return left, false
		}
		time.Sleep(waitTick)
	}
}

// WatchShutdown logs what the stop is waiting for.
//
// Отдельной горутиной, потому что интересен МОМЕНТ сигнала: после возврата
// Start незавершённых уже нет по определению, и логировать там нечего.
func WatchShutdown(ctx context.Context) {
	<-ctx.Done()
	if busy := InFlight(); busy > 0 {
		log.Printf("Stop requested, waiting for %d handler(s), up to %s\n",
			busy, HandlerGrace())
	} else {
		log.Println("Stop requested, nothing in flight")
	}
}
