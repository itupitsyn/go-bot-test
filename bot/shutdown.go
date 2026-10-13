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
// ЧТО УЖЕ РАБОТАЛО БЕЗ НАС. Библиотека дожидается незавершённых обработчиков
// сама: waitUpdates вызывает ProcessUpdate СИНХРОННО в своей горутине, а
// Bot.Start ждёт все горутины. Рабочая горутина не может выйти, пока
// обработчик внутри, поэтому b.Start(ctx) возвращается только после того, как
// текущая генерация досчитана. Своего счётчика ожидания тут не нужно.
//
// ЧЕГО НЕ ХВАТАЛО, И ЭТО ТРИ РАЗНЫЕ ВЕЩИ:
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
//  3. stop_grace_period у Docker по умолчанию ДЕСЯТЬ СЕКУНД, потом SIGKILL.
//     i2v считается около двухсот. Без этой настройки в compose первые два
//     пункта — декорация. Настройка живёт не в этом репозитории.

// handlerGraceDefault — сколько даём обработчику дожить после сигнала.
//
// Пять минут: самое долгое, что бот ждёт синхронно, — это видео, около двухсот
// секунд, плюс запас на выдачу файла в Telegram. Не бесконечность нарочно:
// зависший обработчик не должен держать перезапуск вечно.
const handlerGraceDefault = 5 * time.Minute

// inFlight — сколько обработчиков прямо сейчас в работе.
//
// Не для ожидания (им занята библиотека), а для ЛОГА. Без этой строки
// `docker stop`, висящий три минуты, выглядит как зависший деплой, и его
// добьют руками — ровно в тот момент, когда всё работало правильно.
var inFlight atomic.Int64

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
