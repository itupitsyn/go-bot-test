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

	// Владельца не знаем — поля быть не должно вовсе: наш ноль не должен
	// уехать на сервис как настоящий id.
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

	// Отказ по потолку должен узнаваться наверху через errors.Is, иначе бот
	// покажет «сервер подох» там, где сервер жив и здоров.
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
