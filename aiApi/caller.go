package aiApi

import (
	"errors"
	"strconv"
)

// ErrQueueFull means the service refused the job: this person already has as
// many jobs in progress as it allows at once. It is not a failure but a normal
// answer, and the upper layers must tell it apart from "сервер подох".
var ErrQueueFull = errors.New("the user has too many jobs in flight")

// Caller is who is asking and where to report progress.
//
// The two travel together through the whole generation because both are needed
// all along: the service counts the job cap and builds the round-robin by the
// id, and the callback drives the wait message.
type Caller struct {
	// UserID is the Telegram user id. Zero means "unknown": the service files
	// such jobs under a shared anonymous user.
	UserID int64
	// Progress receives the job's place in the queue on every poll; nil means
	// the queue is not tracked.
	Progress ProgressFunc
}

// userJSON is the owner for a json request body, leading comma included. It is
// an empty string when the owner is unknown: let the service decide what to do
// with a nameless job rather than receive our zero as a real id.
func (c Caller) userJSON() string {
	if c.UserID == 0 {
		return ""
	}

	return `, "user": ` + strconv.FormatInt(c.UserID, 10)
}

// userForm is the owner for a multipart request. An empty string means the
// field is not sent.
func (c Caller) userForm() string {
	if c.UserID == 0 {
		return ""
	}

	return strconv.FormatInt(c.UserID, 10)
}
