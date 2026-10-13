package bot

import (
	"errors"
	"fmt"
	"telebot/aiApi"
	"testing"
)

// Самая тонкая развилка во всей затее: отказ по содержанию и неотвеченный
// фильтр приходят одним кодом 403, но это разные события. Второе — наша
// поломка, и говорить человеку «с таким контентом не работаю» в этом случае
// значит соврать и отправить его переписывать совершенно нормальный промпт.
func TestGenerationErrorTextSplitsRefusalFromOutage(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"взрослое содержание": {
			err:  fmt.Errorf("image: %w", &aiApi.RefusalError{Category: aiApi.RefusalSexual, Where: "prompt"}),
			want: imageRefusalText,
		},
		"несовершеннолетние": {
			err:  fmt.Errorf("edit: %w", &aiApi.RefusalError{Category: aiApi.RefusalMinor, Where: "image"}),
			want: imageRefusalText,
		},
		"фильтр не ответил": {
			err:  fmt.Errorf("image: %w", &aiApi.RefusalError{Category: aiApi.RefusalUnavailable}),
			want: serverDeadText,
		},
		"очередь забита": {
			err:  fmt.Errorf("image: %w", aiApi.ErrQueueFull),
			want: queueFullText,
		},
		"просто поломка": {
			err:  errors.New("connection reset"),
			want: serverDeadText,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			// Без базы GetRandomPhrazeByKey вернёт пусто, и для отказов это
			// ровно та ветка, которую надо проверить: запасная фраза из кода.
			if got := generationErrorText(c.err); got != c.want {
				t.Errorf("текст = %q, ожидался %q", got, c.want)
			}
		})
	}
}

// Запасная фраза обязана быть непустой: пустой текст Telegram не отправит, и
// человек останется вообще без ответа.
func TestImageRefusalFallbackIsNotEmpty(t *testing.T) {
	if imageRefusalText == "" {
		t.Fatal("imageRefusalText пустой")
	}

	if got := imageRefusalPhrase(); got == "" {
		t.Error("imageRefusalPhrase вернул пустую строку без базы")
	}
}
