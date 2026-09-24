package aiApi

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestCallerUserFields(t *testing.T) {
	named := Caller{UserID: 42}
	if got := named.userJSON(); got != `, "user": 42` {
		t.Errorf("userJSON() = %q", got)
	}
	if got := named.userForm(); got != "42" {
		t.Errorf("userForm() = %q", got)
	}

	// The owner is unknown, so the field must be absent altogether: our zero
	// must not reach the service as a real id.
	anon := Caller{}
	if got := anon.userJSON(); got != "" {
		t.Errorf("для безымянного userJSON() = %q, ждали пусто", got)
	}
	if got := anon.userForm(); got != "" {
		t.Errorf("для безымянного userForm() = %q, ждали пусто", got)
	}
}

func TestCheckSubmitStatus(t *testing.T) {
	if err := checkSubmitStatus("image", http.StatusOK, nil); err != nil {
		t.Errorf("на 200 вернулась ошибка: %v", err)
	}

	// A refusal over the cap must be recognizable upstream via errors.Is,
	// otherwise the bot shows "сервер подох" where the server is alive and
	// well.
	err := checkSubmitStatus("t2v", http.StatusTooManyRequests, []byte("full"))
	if !errors.Is(err, ErrQueueFull) {
		t.Errorf("429 дал %v, ждали ErrQueueFull", err)
	}

	err = checkSubmitStatus("i2v", http.StatusInternalServerError, []byte("boom"))
	if err == nil || errors.Is(err, ErrQueueFull) {
		t.Errorf("500 дал %v, ждали обычную ошибку", err)
	}
	if got := fmt.Sprint(err); got == "" {
		t.Error("ошибка без текста")
	}
}
