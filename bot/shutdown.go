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
// context.WithoutCancel отвязывает от отмены, сохраняя значения. Срок жизни
// включается ТОЛЬКО с момента сигнала: пока бот работает штатно, ограничивать
// обработчик нечем.
//
// ТАК БЫЛО НЕ СРАЗУ, И ЭТО СТОИЛО ЛЮДЯМ РЕЗУЛЬТАТОВ. Сначала здесь стоял
// WithTimeout(HandlerGrace()) от начала обработки, то есть пять минут на всю
// работу. Выложено 13.10.2026 в 22:00 — и за следующий час в журнале стенда
// набралось 38 ошибок «context deadline exceeded»: при глубокой очереди видео
// считается дольше пяти минут, после чего КАЖДЫЙ вызов Telegram падал
// мгновенно. Двое платящих людей не получили ни видео, ни даже сообщения об
// ошибке.
//
// Урок общий: потолок на штатную работу — это не страховка, а обрыв. Ждать
// перестаём только тогда, когда нас попросили остановиться.
//
// Важно и то, что отвязан только контекст ОБРАБОТЧИКА. Контекст, по которому
// библиотека решает брать ли новые апдейты, остаётся прежним: новое перестаёт
// приходить сразу, а начатое доживает и успевает отдать результат.
// И ВТОРОЙ РАЗ ТА ЖЕ ОШИБКА, С ДРУГОГО КОНЦА. Здесь стоял `defer cancel()` —
// контекст умирал в момент возврата обработчика. Для самого обработчика это
// незаметно, но часть ответов бот нарочно оставляет работать ПОСЛЕ себя:
// `go utils.SendPhrazes` спит от секунды до пяти между фразами, и обработчик
// возвращается раньше первой отправки. Приз перестал устанавливаться в 22:00
// 13.10.2026, то есть ровно с появлением этого middleware, и в журнале это
// выглядело как «context canceled» в ту же секунду, что и «Prize requested».
// Нашлось только 14.10 днём, когда люди пожаловались.
//
// Поэтому отмена бывает ровно одна: отсрочка после сигнала. В обычной жизни
// контекст живёт столько же, сколько жил до появления middleware, — до конца
// процесса. Отмене по возврату обработчика здесь места нет: кто именно уже
// закончил, middleware не знает, а по возврату не закончила как раз та работа,
// которую мы бережём.
func detachFromShutdown(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		detached, cancel := context.WithCancel(context.WithoutCancel(ctx))

		// finished закрывается по возврату обработчика. Отдельным каналом, а
		// не через detached.Done(), именно потому, что контекст по возврату
		// больше не закрывается, — см. выше.
		finished := make(chan struct{})
		defer close(finished)

		// AfterFunc вызовут в своей горутине в момент сигнала — отсчёт идёт
		// оттуда. Пока сигнала нет, не крутится ничего.
		//
		// Внутри ждём либо отсрочку, либо конца обработки: без второй ветки
		// предупреждение об обрыве печаталось бы и тогда, когда всё давно
		// отдано. А stop() снимает подписку, если обработчик закончил раньше
		// сигнала — то есть в обычной жизни всегда.
		stop := context.AfterFunc(ctx, func() {
			select {
			case <-time.After(HandlerGrace()):
				log.Println("[warn] обработчик не уложился в отсрочку, обрываю")
				cancel()
			case <-finished:
			}
		})
		defer stop()

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
