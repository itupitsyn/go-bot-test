package bot

import (
	"testing"
)

func TestThumbnailURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		token   string
		want    string
	}{
		{"plain", "https://bot.example.com", "sekret", "https://bot.example.com/img/sekret"},
		{"trailing slash", "https://bot.example.com/", "sekret", "https://bot.example.com/img/sekret"},
		// Without a public address there is nowhere to point, and an inline
		// result has to go without a preview rather than with a broken link.
		{"no public base", "", "sekret", ""},
		// A picture saved before previews existed carries no token until the
		// backfill reaches it.
		{"no token", "https://bot.example.com", "", ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("PUBLIC_BASE_URL", test.baseURL)

			if got := thumbnailURL(test.token); got != test.want {
				t.Errorf("want %q, got %q", test.want, got)
			}
		})
	}
}
