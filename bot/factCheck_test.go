package bot

import (
	"fmt"
	"strings"
	"telebot/aiApi"
	"testing"
	"time"
)

func TestIsFactCheckCommand(t *testing.T) {
	cases := map[string]bool{
		"проверь":       true,
		"проверь?":      true,
		"  проверь  ":   true,
		"fact check":    true,
		"factcheck":     true,
		"проверь это":   false,
		"проверь почту": false,
		"а проверь":     false,
		"проверьте":     false,
		"что тут":       false,
	}
	for text, want := range cases {
		if got := isFactCheckCommand(text); got != want {
			t.Errorf("isFactCheckCommand(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestFactCheckFailTextTellsFailuresApart(t *testing.T) {
	cases := map[error]string{
		fmt.Errorf("%w: status 502", aiApi.ErrSearchUnavailable): searchDownText,
		fmt.Errorf("%w: captcha", aiApi.ErrSearchEnginesDown):    enginesDownText,
		fmt.Errorf("%w: next turn in 70s", aiApi.ErrSearchBusy):  searchBusyText,
		fmt.Errorf("llm request failed with status 500"):         serverDeadText,
	}
	for err, want := range cases {
		if got := factCheckFailText(err); got != want {
			t.Errorf("factCheckFailText(%v) = %q, want %q", err, got, want)
		}
	}
	if searchDownText == enginesDownText {
		t.Error("the two search failures must read differently")
	}
}

func TestFormatFactCheck(t *testing.T) {
	notFound := formatFactCheck(&aiApi.FactCheck{Verdict: aiApi.FactNotFound})
	if notFound != factNotFoundText {
		t.Errorf("not found = %q", notFound)
	}

	confirmed := formatFactCheck(&aiApi.FactCheck{Verdict: aiApi.FactConfirmed, Evidence: []aiApi.FactEvidence{
		{Quote: "Talarico has won", URL: "https://apnews.com/a", Supports: true},
	}})
	if !strings.HasPrefix(confirmed, factConfirmedText) || !strings.Contains(confirmed, "«Talarico has won»\nhttps://apnews.com/a") {
		t.Errorf("confirmed = %q", confirmed)
	}
	if strings.Contains(confirmed, "За:") {
		t.Errorf("sides are only for conflicting sources: %q", confirmed)
	}

	conflicting := formatFactCheck(&aiApi.FactCheck{Verdict: aiApi.FactConflicting, Evidence: []aiApi.FactEvidence{
		{Quote: "yes", URL: "https://a", Supports: true},
		{Quote: "no", URL: "https://b"},
	}})
	if !strings.Contains(conflicting, "За: «yes»") || !strings.Contains(conflicting, "Против: «no»") {
		t.Errorf("conflicting = %q", conflicting)
	}

	many := &aiApi.FactCheck{Verdict: aiApi.FactRefuted}
	for i := 0; i < 5; i++ {
		many.Evidence = append(many.Evidence, aiApi.FactEvidence{Quote: strings.Repeat("я", 500), URL: fmt.Sprintf("https://%d", i)})
	}
	text := formatFactCheck(many)
	if strings.Count(text, "https://") != maxEvidenceShown || strings.Count(text, "…") != maxEvidenceShown {
		t.Errorf("want %d clipped quotes, got %q", maxEvidenceShown, text)
	}
}

func TestFactCheckGateBudget(t *testing.T) {
	gate := &factCheckGate{cache: map[string]factCacheEntry{}, asked: map[int64][]time.Time{}, running: map[int64]bool{}}
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

	if !gate.admit(1, now) {
		t.Fatal("first check refused")
	}
	if gate.admit(1, now) {
		t.Error("a second check while the first runs must wait")
	}
	gate.done(1)

	for i := 1; i < factCheckPerHour; i++ {
		if !gate.admit(1, now.Add(time.Duration(i)*time.Minute)) {
			t.Fatalf("check %d refused", i+1)
		}
		gate.done(1)
	}
	if gate.admit(1, now.Add(30*time.Minute)) {
		t.Error("over the hourly budget")
	}
	if !gate.admit(2, now.Add(30*time.Minute)) {
		t.Error("the budget is per person")
	}
	gate.done(2)
	if !gate.admit(1, now.Add(61*time.Minute)) {
		t.Error("an hour later the budget is back")
	}
}

func TestFactCheckGateCache(t *testing.T) {
	gate := &factCheckGate{cache: map[string]factCacheEntry{}, asked: map[int64][]time.Time{}, running: map[int64]bool{}}
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	result := &aiApi.FactCheck{Verdict: aiApi.FactConfirmed}

	gate.remember("Земля  круглая", result, now)
	if gate.cached("земля круглая", now.Add(time.Minute)) != result {
		t.Error("case and spacing must not matter")
	}
	if gate.cached("земля круглая", now.Add(factCheckCacheTTL+time.Minute)) != nil {
		t.Error("an expired answer must not be served: news moves")
	}

	for i := 0; i < factCheckCacheSize+5; i++ {
		gate.remember(fmt.Sprintf("claim %d", i), result, now.Add(time.Duration(i)*time.Second))
	}
	if len(gate.cache) > factCheckCacheSize {
		t.Errorf("cache grew to %d", len(gate.cache))
	}
}
