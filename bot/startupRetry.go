package bot

import (
	"context"
	"log"
	"time"
)

// Startup calls to Telegram are retried instead of killing the process.
//
// WHY. The bot talks to our own telegram-bot-api (see TELEGRAM_API_URL) and
// that path runs over a home link. On 29.10.2026 the link was out for three
// hours. A *running* bot rides such an outage out by itself: it logs a
// getUpdates error every thirty-five seconds and picks up where it left off
// once the link is back. A *restart* in the middle of one used to be fatal —
// bot.New calls getMe, the error reached panic(), systemd started a new
// process nine seconds later and the cycle repeated 1292 times. The bot was
// down the whole three hours even though nothing was wrong with it, and the
// one thing that would have helped — doing nothing — is the opposite of what
// anyone does when a bot goes quiet.
//
// WHY THE BUDGET IS FINITE. Waiting forever would hide a real problem, such
// as a revoked token: that answers at once with 401 instead of timing out, and
// a bot stuck in a silent retry loop looks exactly like a bot that works.
// Running out of tries exits, systemd starts a new process and the waiting
// begins again — so a three-hour outage now costs about seven restarts instead
// of 1292, and the bot is serving again within a minute of the link returning.
//
// These are vars rather than consts only so that the tests do not have to
// sleep through the real pauses.
var (
	startupTries     = 30
	startupFirstWait = 2 * time.Second
	startupMaxWait   = time.Minute
)

// startupWait is how long to wait after try number try (1-based) has failed.
// The pause doubles up to startupMaxWait, so a blip costs a couple of seconds
// while a long outage is not hammered once a minute is reached.
func startupWait(try int) time.Duration {
	wait := startupFirstWait

	for i := 1; i < try; i++ {
		if wait >= startupMaxWait {
			break
		}
		wait *= 2
	}

	if wait > startupMaxWait {
		return startupMaxWait
	}

	return wait
}

// keepTrying runs a startup call until it succeeds, the tries run out or the
// context is cancelled, and returns the last error. The name of the call goes
// into the log so that a slow start says which step is waiting; the error is
// logged through the standard logger on purpose, because that is where the
// token gets masked (see utils.HideSecretsInLogs).
func keepTrying(ctx context.Context, what string, call func() error) error {
	var err error

	for try := 1; try <= startupTries; try++ {
		if err = call(); err == nil {
			if try > 1 {
				log.Printf("%s succeeded on try %d", what, try)
			}

			return nil
		}

		log.Printf("[warn] %s failed (try %d of %d): %v", what, try, startupTries, err)

		if try == startupTries {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(startupWait(try)):
		}
	}

	return err
}
