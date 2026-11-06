package aiApi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// factWorld is a fake search server and a fake llm around one checkFact call.
// The llm tells the three questions apart by their system prompts.
type factWorld struct {
	mu          sync.Mutex
	searchCode  int
	searchBody  string
	evidence    string
	bothCanBe   string
	evidenceRan bool
	rechecks    int
}

func (world *factWorld) search(w http.ResponseWriter, r *http.Request) {
	if world.searchCode != 0 {
		w.WriteHeader(world.searchCode)
		return
	}
	fmt.Fprint(w, world.searchBody)
}

func (world *factWorld) llm(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var request struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	json.Unmarshal(body, &request)
	system := request.Messages[0].Content

	world.mu.Lock()
	defer world.mu.Unlock()

	answer := `{"query": "talarico paxton"}`
	switch {
	case strings.Contains(system, "Не оценивай утверждение"):
		world.evidenceRan = true
		answer = world.evidence
	case strings.Contains(system, "Может ли быть так"):
		world.rechecks++
		answer = `{"reason": "x", "both_can_be_true": ` + world.bothCanBe + `}`
	}
	fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, answer)
}

const twoResults = `{"results": [
 {"title": "AP", "url": "https://apnews.com/a", "content": "Democrat James Talarico has won the Texas Senate race against Ken Paxton.", "engines": ["yandex"]},
 {"title": "Blog", "url": "https://blog.example/b", "content": "Ken Paxton won the Texas Senate race, the blog claims.", "engines": ["duckduckgo"]}
], "unresponsive_engines": []}`

func runFactCheck(t *testing.T, world *factWorld) (*FactCheck, error) {
	t.Helper()

	search := httptest.NewServer(http.HandlerFunc(world.search))
	t.Cleanup(search.Close)
	llm := httptest.NewServer(http.HandlerFunc(world.llm))
	t.Cleanup(llm.Close)
	t.Setenv("FACTCHECK_SEARCH_URL", search.URL+"/")
	t.Setenv("AI_LLM_URL", llm.URL)
	noPacing(t)

	return checkFact("Talarico defeated Paxton in Texas.", time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC))
}

// noPacing swaps the shared search queue for one without pauses, so the tests
// do not wait ten seconds between searches.
func noPacing(t *testing.T) {
	t.Helper()
	saved := searchTurns
	searchTurns = newSearchPacer(0, 0, time.Minute)
	searchTurns.sleep = func(time.Duration) {}
	t.Cleanup(func() { searchTurns = saved })
}

// fakeClock is a clock that moves only when someone sleeps.
type fakeClock struct {
	mu    sync.Mutex
	at    time.Time
	slept []time.Duration
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *fakeClock) sleep(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.slept = append(c.slept, d)
}

func TestSearchPacerSpacesTurns(t *testing.T) {
	clock := &fakeClock{at: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)}
	pacer := newSearchPacer(10*time.Second, 2*time.Minute, time.Minute)
	pacer.now, pacer.sleep = clock.now, clock.sleep

	// Three people at the same moment: the first goes at once, the others
	// wait 10 and 20 s.
	for i := 0; i < 3; i++ {
		if err := pacer.wait(); err != nil {
			t.Fatal(err)
		}
	}
	if len(clock.slept) != 2 || clock.slept[0] != 10*time.Second || clock.slept[1] != 20*time.Second {
		t.Fatalf("slept %v, want [10s 20s]", clock.slept)
	}

	// Long after, the queue is empty again: no wait.
	clock.at = clock.at.Add(time.Hour)
	clock.slept = nil
	if err := pacer.wait(); err != nil || len(clock.slept) != 0 {
		t.Fatalf("an idle queue must not make anyone wait: %v %v", err, clock.slept)
	}
}

func TestSearchPacerRefusesALongQueue(t *testing.T) {
	clock := &fakeClock{at: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)}
	pacer := newSearchPacer(10*time.Second, 2*time.Minute, time.Minute)
	pacer.now, pacer.sleep = clock.now, clock.sleep

	// With a one-minute limit, turns at 0, 10, ... 60 s fit; the eighth would wait 70 s.
	for i := 0; i < 7; i++ {
		if err := pacer.wait(); err != nil {
			t.Fatalf("turn %d: %v", i+1, err)
		}
	}
	err := pacer.wait()
	if !errors.Is(err, ErrSearchBusy) {
		t.Fatalf("want ErrSearchBusy, got %v", err)
	}

	// A refused check books nothing: the next one after the queue moves gets
	// the turn the refused one did not take.
	clock.at = clock.at.Add(10 * time.Second)
	if err := pacer.wait(); err != nil {
		t.Fatalf("after the queue moved: %v", err)
	}
}

func found(engines ...string) []searchResult {
	results := make([]searchResult, len(engines))
	for i, engine := range engines {
		results[i] = searchResult{URL: "https://x", Content: "x", Engines: []string{engine}}
	}
	return results
}

func TestSearchPacerAdapts(t *testing.T) {
	clock := &fakeClock{at: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)}
	pacer := newSearchPacer(10*time.Second, 2*time.Minute, 5*time.Minute)
	pacer.now, pacer.sleep = clock.now, clock.sleep
	ddg := [][]string{{"duckduckgo", "CAPTCHA"}}

	// The first DuckDuckGo captcha is news: pressure.
	pacer.observe(found("yandex", "yandex"), ddg, false)
	if pacer.interval != 20*time.Second {
		t.Fatalf("first captcha: interval %s, want 20s", pacer.interval)
	}
	// The same captcha again is DuckDuckGo's usual state, not pressure: calm.
	pacer.observe(found("yandex"), ddg, false)
	if pacer.interval != 10*time.Second {
		t.Fatalf("a chronic captcha must not keep the interval up: %s", pacer.interval)
	}
	for i := 0; i < 5; i++ {
		pacer.observe(found("yandex"), ddg, false)
	}
	if pacer.interval != 10*time.Second {
		t.Fatalf("never below base: %s", pacer.interval)
	}

	// Yandex worked and now times out: pressure, and the booked turn moves.
	pacer.next = clock.at
	pacer.observe(found("bing news"), [][]string{{"duckduckgo", "CAPTCHA"}, {"yandex", "timeout"}}, false)
	if pacer.interval != 20*time.Second || !pacer.next.Equal(clock.at.Add(20*time.Second)) {
		t.Fatalf("yandex failing: interval %s next %s", pacer.interval, pacer.next.Sub(clock.at))
	}

	// Everything down is pressure even for engines already failing; the
	// interval stops at the cap.
	for i := 0; i < 6; i++ {
		pacer.observe(nil, [][]string{{"duckduckgo", "CAPTCHA"}, {"yandex", "CAPTCHA"}}, true)
	}
	if pacer.interval != 2*time.Minute {
		t.Fatalf("cap: %s", pacer.interval)
	}

	// Yandex is back: calm searches halve the interval down to base.
	want := []time.Duration{time.Minute, 30 * time.Second, 15 * time.Second, 10 * time.Second}
	for i, w := range want {
		pacer.observe(found("yandex"), ddg, false)
		if pacer.interval != w {
			t.Fatalf("recovery step %d: %s, want %s", i+1, pacer.interval, w)
		}
	}
}

func TestFactCheckConfirmsOnlyOnARealQuote(t *testing.T) {
	world := &factWorld{searchBody: twoResults,
		evidence: `{"evidence": [{"source": 1, "quote": "James Talarico has won the Texas Senate race", "relation": "supports"}]}`}
	result, err := runFactCheck(t, world)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != FactConfirmed || len(result.Evidence) != 1 || result.Evidence[0].URL != "https://apnews.com/a" {
		t.Fatalf("got %+v", result)
	}
	if world.rechecks != 0 {
		t.Errorf("support needs no recheck, got %d", world.rechecks)
	}
}

func TestFactCheckDropsAnInventedQuote(t *testing.T) {
	world := &factWorld{searchBody: twoResults,
		evidence: `{"evidence": [{"source": 1, "quote": "Talarico won by a landslide of twenty points", "relation": "supports"}]}`}
	result, err := runFactCheck(t, world)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != FactNotFound || len(result.Evidence) != 0 {
		t.Fatalf("a quote that is not in the source must not count: %+v", result)
	}
}

func TestFactCheckDropsAQuoteFromTheWrongSource(t *testing.T) {
	world := &factWorld{searchBody: twoResults,
		evidence: `{"evidence": [{"source": 2, "quote": "James Talarico has won the Texas Senate race", "relation": "supports"}]}`}
	result, err := runFactCheck(t, world)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != FactNotFound {
		t.Fatalf("the quote is in source 1, not 2: %+v", result)
	}
}

func TestFactCheckRefutesOnlyWhenTheRecheckAgrees(t *testing.T) {
	evidence := `{"evidence": [{"source": "2", "quote": "Ken Paxton won the Texas Senate race", "relation": "contradicts"}]}`

	world := &factWorld{searchBody: twoResults, evidence: evidence, bothCanBe: "false"}
	result, err := runFactCheck(t, world)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != FactRefuted || world.rechecks != 1 {
		t.Fatalf("recheck said they exclude each other: %+v, rechecks %d", result, world.rechecks)
	}

	world = &factWorld{searchBody: twoResults, evidence: evidence, bothCanBe: "true"}
	result, err = runFactCheck(t, world)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != FactNotFound {
		t.Fatalf("recheck said both can be true, so no refutation: %+v", result)
	}

	world = &factWorld{searchBody: twoResults, evidence: evidence, bothCanBe: `"maybe"`}
	result, err = runFactCheck(t, world)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != FactNotFound {
		t.Fatalf("an unreadable recheck must fall to the harmless side: %+v", result)
	}
}

func TestFactCheckConflictingWhenBothSidesHold(t *testing.T) {
	world := &factWorld{searchBody: twoResults, bothCanBe: "false", evidence: `{"evidence": [
		{"source": 1, "quote": "James Talarico has won the Texas Senate race", "relation": "supports"},
		{"source": 2.0, "quote": "Ken Paxton won the Texas Senate race", "relation": "contradicts"}]}`}
	result, err := runFactCheck(t, world)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != FactConflicting || len(result.Evidence) != 2 {
		t.Fatalf("got %+v", result)
	}
}

func TestFactCheckSearchServerDown(t *testing.T) {
	world := &factWorld{searchCode: http.StatusBadGateway}
	_, err := runFactCheck(t, world)
	if !errors.Is(err, ErrSearchUnavailable) {
		t.Fatalf("want ErrSearchUnavailable, got %v", err)
	}
	if world.evidenceRan {
		t.Error("nothing was searched, the model must not be asked")
	}

	world = &factWorld{searchBody: "<html>oops</html>"}
	if _, err := runFactCheck(t, world); !errors.Is(err, ErrSearchUnavailable) {
		t.Fatalf("garbage from the server is the server's fault, got %v", err)
	}
}

func TestFactCheckSearchServerUnreachable(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc((&factWorld{}).llm))
	t.Cleanup(llm.Close)
	t.Setenv("AI_LLM_URL", llm.URL)
	noPacing(t)

	closed := httptest.NewServer(http.NotFoundHandler())
	address := closed.URL
	closed.Close()
	t.Setenv("FACTCHECK_SEARCH_URL", address)

	if _, err := checkFact("x", time.Now()); !errors.Is(err, ErrSearchUnavailable) {
		t.Fatalf("want ErrSearchUnavailable, got %v", err)
	}

	t.Setenv("FACTCHECK_SEARCH_URL", "")
	if _, err := checkFact("x", time.Now()); !errors.Is(err, ErrSearchUnavailable) {
		t.Fatalf("an unset address is an unavailable search, got %v", err)
	}
}

func TestFactCheckEnginesDownIsNotNotFound(t *testing.T) {
	world := &factWorld{searchBody: `{"results": [], "unresponsive_engines": [["yandex", "CAPTCHA"], ["duckduckgo", "access denied"]]}`}
	_, err := runFactCheck(t, world)
	if !errors.Is(err, ErrSearchEnginesDown) {
		t.Fatalf("want ErrSearchEnginesDown, got %v", err)
	}
}

func TestFactCheckNothingFoundIsNotFound(t *testing.T) {
	world := &factWorld{searchBody: `{"results": [], "unresponsive_engines": []}`}
	result, err := runFactCheck(t, world)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != FactNotFound || world.evidenceRan {
		t.Fatalf("engines answered with nothing: not found, without asking the model: %+v", result)
	}
}

func TestFactCheckOneEngineDownStillSearches(t *testing.T) {
	body := strings.Replace(twoResults, `"unresponsive_engines": []`, `"unresponsive_engines": [["duckduckgo", "CAPTCHA"]]`, 1)
	world := &factWorld{searchBody: body,
		evidence: `{"evidence": [{"source": 1, "quote": "James Talarico has won the Texas Senate race", "relation": "supports"}]}`}
	result, err := runFactCheck(t, world)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != FactConfirmed {
		t.Fatalf("one engine failing must not stop the check: %+v", result)
	}
}

func TestTodayLineIsMoscowDate(t *testing.T) {
	// 22:30 UTC on the 5th is already the 6th in Moscow.
	got := todayLine(time.Date(2026, 11, 5, 22, 30, 0, 0, time.UTC))
	if got != "Сегодня 6 ноября 2026 года. " {
		t.Errorf("todayLine = %q", got)
	}
}

func TestNormalizedMatchesAcrossPunctuation(t *testing.T) {
	snippet := normalized("(2026-11-03) AP — Democrat James Talarico has won, the Texas Senate race!")
	if !strings.Contains(snippet, normalized("James Talarico has won the Texas  Senate race")) {
		t.Error("spacing and punctuation must not matter")
	}
}
