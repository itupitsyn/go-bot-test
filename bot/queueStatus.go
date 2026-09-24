package bot

import (
	"errors"
	"fmt"
	"telebot/aiApi"
	"time"
)

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
func generationErrorText(err error) string {
	if errors.Is(err, aiApi.ErrQueueFull) {
		return queueFullText
	}

	return serverDeadText
}
