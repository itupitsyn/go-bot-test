package aiApi

import (
	"errors"
	"net/http"
	"testing"
)

func TestCheckSubmitStatusCarriesRefusalCategory(t *testing.T) {
	body := []byte(`{"detail":{"refusal":"minor","where":"prompt"}}`)

	err := checkSubmitStatus("image", http.StatusForbidden, body)
	if err == nil {
		t.Fatal("403 должен давать ошибку")
	}

	if !errors.Is(err, ErrRefused) {
		t.Errorf("errors.Is(err, ErrRefused) = false, а должно пройти через обёртку fmt.Errorf")
	}

	var refusal *RefusalError
	if !errors.As(err, &refusal) {
		t.Fatal("errors.As не достал *RefusalError")
	}

	if refusal.Category != RefusalMinor {
		t.Errorf("категория = %q, ожидалась %q", refusal.Category, RefusalMinor)
	}

	if refusal.Where != "prompt" {
		t.Errorf("where = %q, ожидалось prompt", refusal.Where)
	}
}

// Отказ не должен превращаться в «сервер умер» только потому, что тело ответа
// оказалось не таким, как мы ждали: код 403 уже сказал главное.
func TestParseRefusalSurvivesBadBody(t *testing.T) {
	for name, body := range map[string]string{
		"мусор":          "not json at all",
		"пусто":          "",
		"без поля":       `{"detail":{}}`,
		"чужая форма":    `{"message":"forbidden"}`,
		"detail строкой": `{"detail":"forbidden"}`,
	} {
		t.Run(name, func(t *testing.T) {
			err := checkSubmitStatus("edit", http.StatusForbidden, []byte(body))
			if !errors.Is(err, ErrRefused) {
				t.Fatalf("тело %q перестало считаться отказом", body)
			}

			var refusal *RefusalError
			if !errors.As(err, &refusal) {
				t.Fatal("нет *RefusalError")
			}

			if refusal.Category == "" {
				t.Error("категория пустая — вызывающий не сможет отличить её от unavailable")
			}
		})
	}
}

// Прежние коды не должны поехать: 403 добавлен рядом, а не вместо.
func TestCheckSubmitStatusKeepsOtherCodes(t *testing.T) {
	if err := checkSubmitStatus("image", http.StatusOK, nil); err != nil {
		t.Errorf("200 дал ошибку: %v", err)
	}

	if err := checkSubmitStatus("image", http.StatusTooManyRequests, nil); !errors.Is(err, ErrQueueFull) {
		t.Errorf("429 должен давать ErrQueueFull, дал %v", err)
	}

	if err := checkSubmitStatus("image", http.StatusUnauthorized, nil); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("401 должен давать ErrUnauthorized, дал %v", err)
	}

	err := checkSubmitStatus("image", http.StatusInternalServerError, []byte("boom"))
	if errors.Is(err, ErrRefused) {
		t.Error("500 не должен считаться отказом по содержанию")
	}
}
