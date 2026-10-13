package bot

import (
	"errors"
	"fmt"
	"strings"
	"telebot/aiApi"
	"telebot/raffleLogic"
	"time"
)

// imageRefusalKey is the phrase key the admin panel edits. Keep it in step
// with PHRAZE_KEY_OPTIONS in kalovrat-admin/lib/constants.ts — a key missing
// from that list cannot be picked in the UI, so the phrases become unreachable
// even though the bot would read them.
const imageRefusalKey = "image_refusal"

// formatQueueStatus says what to expect. While the job is queued, how many
// people are ahead; as soon as it is picked up (or nobody was ahead in the
// first place), the queue is over and only a plain "жди" is left.
//
// An empty queue must not be confused with "one ahead": a free GPU does not
// pick up the job instantly, and in that gap the bot must not announce a queue
// consisting of itself.
func formatQueueStatus(status aiApi.QueueStatus, waitPhrase string) string {
	if status.Running || status.Ahead == 0 {
		return waitPhrase
	}

	return fmt.Sprintf("Ты %d-й в очереди, %s", status.Ahead+1, formatEta(status.ETA))
}

// formatEta turns the wait estimate into human words. Minutes are rounded: the
// precision here is illusory, the one waiting cares about the order of
// magnitude.
func formatEta(eta time.Duration) string {
	minutes := int(eta.Round(time.Minute).Minutes())
	if minutes < 1 {
		return "уже скоро"
	}

	return fmt.Sprintf("минут %d", minutes)
}

// generationErrorText is what to show instead of the result. A refusal over the
// cap is not a failure: talking about a dead server when the person has simply
// generated too much would be a lie.
//
// The same reasoning splits the content filter in two. A declined request is
// not breakage either — but a filter that did not answer IS, and the service
// marks that case as RefusalUnavailable. Reporting it as "I don't work with
// such content" would blame the person for our outage and send them off
// rewriting a prompt that was fine.
func generationErrorText(err error) string {
	if errors.Is(err, aiApi.ErrQueueFull) {
		return queueFullText
	}

	var refusal *aiApi.RefusalError
	if errors.As(err, &refusal) {
		if refusal.Category == aiApi.RefusalUnavailable {
			return serverDeadText
		}

		return imageRefusalPhrase()
	}

	return serverDeadText
}

// imageRefusalPhrase takes the wording from the admin panel and falls back to
// the built-in constant.
//
// The censored set is asked for on purpose (false): a refusal is exactly the
// moment to be polite, and the rude variants have no business here even in a
// chat that allows them.
func imageRefusalPhrase() string {
	phrazes := raffleLogic.GetRandomPhrazeByKey(imageRefusalKey, false)
	if len(phrazes) == 0 {
		return imageRefusalText
	}

	parts := make([]string, 0, len(phrazes))
	for _, phraze := range phrazes {
		if phraze.Value != "" {
			parts = append(parts, phraze.Value)
		}
	}

	if len(parts) == 0 {
		return imageRefusalText
	}

	// A grouped phrase arrives as several rows meant to be read in order.
	return strings.Join(parts, "\n")
}
