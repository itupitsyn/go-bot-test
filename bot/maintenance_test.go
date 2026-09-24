package bot

import (
	"telebot/model"
	"testing"
	"time"
)

func TestMaintenanceText(t *testing.T) {
	now := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	at := func(t time.Time) *time.Time { return &t }

	cases := []struct {
		name   string
		endsAt *time.Time
		want   string
	}{
		{"no end", nil, "Нейронки на профилактике, скоро вернёмся"},
		{"today", at(time.Date(2026, 9, 16, 18, 30, 0, 0, time.UTC)), "Нейронки на профилактике, вернёмся сегодня в 18:30 UTC"},
		{"tomorrow", at(time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC)), "Нейронки на профилактике, вернёмся завтра в 09:00 UTC"},
		{"later", at(time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)), "Нейронки на профилактике, вернёмся 20.09 в 09:00 UTC"},
		// A deadline set in another time zone is still stated in UTC.
		{"other zone", at(time.Date(2026, 9, 17, 1, 0, 0, 0, time.FixedZone("MSK", 3*60*60))), "Нейронки на профилактике, вернёмся сегодня в 22:00 UTC"},
	}

	for _, c := range cases {
		got := maintenanceText(&model.AiMaintenance{IsEnabled: true, EndsAt: c.endsAt}, now)
		if got != c.want {
			t.Errorf("%s: want %q, got %q", c.name, c.want, got)
		}
	}
}
