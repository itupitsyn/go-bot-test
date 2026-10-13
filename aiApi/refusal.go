package aiApi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ErrRefused means the generation service's content filter turned the request
// down before it reached a card. It is neither a failure nor a queue problem,
// so the words shown to the person have to differ from both.
var ErrRefused = errors.New("the generation service refused the content")

// Refusal categories, exactly as the service spells them.
const (
	// RefusalSexual: adult material. Worth a polite phrase and nothing more.
	RefusalSexual = "sexual"
	// RefusalMinor: the request involved a minor. A different weight of event
	// entirely — this one is worth looking at by hand.
	RefusalMinor = "minor"
	// RefusalUnavailable: the filter itself did not answer and the service
	// refused on the safe side. OUR breakage, not the person's content, so it
	// must never be reported as "I don't work with such content" — that would
	// be a lie and would send them off rewriting a perfectly fine prompt.
	RefusalUnavailable = "unavailable"
)

// RefusalError carries the category so the caller can tell those three apart.
type RefusalError struct {
	Category string
	Where    string // prompt | image
}

func (e *RefusalError) Error() string {
	return fmt.Sprintf("content refused: %s (%s)", e.Category, e.Where)
}

// Unwrap lets errors.Is(err, ErrRefused) work through the fmt.Errorf wrapping
// that checkSubmitStatus adds, while errors.As still gets the category.
func (e *RefusalError) Unwrap() error { return ErrRefused }

// parseRefusal reads the service's body: {"detail":{"refusal":"sexual",
// "where":"prompt"}}.
//
// A body we cannot parse is still a refusal. The status code has already said
// so, and falling back to "the server died" would tell the person something
// untrue about a request that was simply declined.
func parseRefusal(body []byte) *RefusalError {
	var payload struct {
		Detail struct {
			Refusal string `json:"refusal"`
			Where   string `json:"where"`
		} `json:"detail"`
	}

	if err := json.Unmarshal(body, &payload); err != nil || payload.Detail.Refusal == "" {
		return &RefusalError{Category: "unknown"}
	}

	return &RefusalError{Category: payload.Detail.Refusal, Where: payload.Detail.Where}
}

// refusalStatus is the code the service answers with. Kept next to the parser
// so the two cannot drift apart.
const refusalStatus = http.StatusForbidden
