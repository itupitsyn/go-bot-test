package bot

import (
	"context"
	"errors"
	"testing"
	"time"
)

// shortenStartupWaits makes the pauses negligible, so that a test measures
// behaviour instead of sleeping through half an hour of real backoff.
func shortenStartupWaits(t *testing.T, tries int) {
	t.Helper()

	oldTries, oldFirst, oldMax := startupTries, startupFirstWait, startupMaxWait
	startupTries = tries
	startupFirstWait = time.Millisecond
	startupMaxWait = 2 * time.Millisecond

	t.Cleanup(func() {
		startupTries, startupFirstWait, startupMaxWait = oldTries, oldFirst, oldMax
	})
}

func TestKeepTryingRidesOutAnOutage(t *testing.T) {
	// This is 29.10.2026 itself: the link is down when the process starts and
	// comes back a little later. The bot must come up on its own, without a
	// restart and without anybody noticing.
	shortenStartupWaits(t, 5)

	calls := 0
	err := keepTrying(context.Background(), "getMe", func() error {
		calls++
		if calls < 3 {
			return errors.New("context deadline exceeded")
		}

		return nil
	})

	if err != nil {
		t.Fatalf("expected the third try to succeed, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected to stop calling once it worked, made %d calls", calls)
	}
}

func TestKeepTryingGivesUpAfterTheBudget(t *testing.T) {
	// The budget has to run out, or a revoked token — which answers at once
	// instead of timing out — would leave the bot silently retrying forever
	// and looking healthy from the outside.
	shortenStartupWaits(t, 4)

	dead := errors.New("Unauthorized")
	calls := 0
	err := keepTrying(context.Background(), "getMe", func() error {
		calls++

		return dead
	})

	if !errors.Is(err, dead) {
		t.Fatalf("expected the last error back, got %v", err)
	}
	if calls != 4 {
		t.Fatalf("expected exactly the budget of tries, made %d", calls)
	}
}

func TestKeepTryingStopsOnShutdown(t *testing.T) {
	// SIGTERM during the wait must not be held hostage by the backoff: the
	// bot is stopped, there is nobody left to come up for.
	shortenStartupWaits(t, 100)
	startupFirstWait = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	done := make(chan error, 1)

	go func() {
		done <- keepTrying(ctx, "getMe", func() error {
			calls++
			cancel()

			return errors.New("i/o timeout")
		})
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected the cancellation back, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("keepTrying sat in the backoff instead of honouring the cancellation")
	}

	if calls != 1 {
		t.Fatalf("expected no call after the cancellation, made %d", calls)
	}
}

func TestStartupWaitGrowsAndIsCapped(t *testing.T) {
	if got := startupWait(1); got != 2*time.Second {
		t.Fatalf("first wait: expected 2s, got %v", got)
	}
	if got := startupWait(3); got != 8*time.Second {
		t.Fatalf("third wait: expected 8s, got %v", got)
	}

	// Past the cap the pause stops growing — including far past it, where a
	// plain doubling would overflow into nonsense.
	for _, try := range []int{6, 7, 30, 1000} {
		if got := startupWait(try); got != startupMaxWait {
			t.Fatalf("wait %d: expected the cap %v, got %v", try, startupMaxWait, got)
		}
	}
}

func TestStartupBudgetIsAboutHalfAnHour(t *testing.T) {
	// The comment in startupRetry.go promises that a three-hour outage costs
	// about seven restarts rather than 1292 panics. That only holds while one
	// process is patient for roughly half an hour; this test is what keeps the
	// promise honest if somebody tunes the constants.
	var total time.Duration
	for try := 1; try < startupTries; try++ {
		total += startupWait(try)
	}

	if total < 20*time.Minute || total > 35*time.Minute {
		t.Fatalf("expected one process to wait 20-35 minutes in total, it waits %v", total)
	}
}
