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

// factWorld is a fake SearXNG, a fake Brave API and a fake llm around one
// checkFact call. The llm tells its questions apart by their system prompts.
type factWorld struct {
	mu sync.Mutex

	plan      string // the plan answer; empty means "checkable, one query"
	evidence  []string
	bothCanBe string
	refine    string
	primary   string

	searxCode   int
	searxBody   string
	searxBodies []string // per search, in order; overrides searxBody
	searxCalls  int

	braveCode  int
	braveBody  string
	braveCalls int
	braveQuery []string

	evidenceCalls int
	evidenceUsers []string
	rechecks      int
	refines       int
}

func (w *factWorld) searx(rw http.ResponseWriter, r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.searxCalls++
	if w.searxCode != 0 {
		rw.WriteHeader(w.searxCode)
		return
	}
	body := w.searxBody
	if len(w.searxBodies) > 0 {
		body = w.searxBodies[0]
		if len(w.searxBodies) > 1 {
			w.searxBodies = w.searxBodies[1:]
		}
	}
	fmt.Fprint(rw, body)
}

func (w *factWorld) brave(rw http.ResponseWriter, r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.braveCalls++
	w.braveQuery = append(w.braveQuery, r.URL.RawQuery)
	if r.Header.Get("X-Subscription-Token") != "test-key" {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}
	if w.braveCode != 0 {
		rw.WriteHeader(w.braveCode)
		return
	}
	body := w.braveBody
	if body == "" {
		body = `{"web": {"results": []}}`
	}
	fmt.Fprint(rw, body)
}

func (w *factWorld) llm(rw http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var request struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	json.Unmarshal(body, &request)
	system, user := request.Messages[0].Content, request.Messages[1].Content

	w.mu.Lock()
	defer w.mu.Unlock()

	answer := `{}`
	switch {
	case strings.Contains(system, "с просьбой проверить его"):
		answer = w.plan
		if answer == "" {
			answer = `{"checkable": true, "fresh": false, "queries": ["talarico paxton"]}`
		}
	case strings.Contains(system, "Не оценивай утверждение"):
		w.evidenceUsers = append(w.evidenceUsers, user)
		answer = `{"evidence": []}`
		if w.evidenceCalls < len(w.evidence) {
			answer = w.evidence[w.evidenceCalls]
		}
		w.evidenceCalls++
	case strings.Contains(system, "Может ли быть так"):
		w.rechecks++
		answer = `{"reason": "x", "both_can_be_true": ` + w.bothCanBe + `}`
	case strings.Contains(system, "Первый поиск не дал"):
		w.refines++
		answer = w.refine
		if answer == "" {
			answer = `{"query": ""}`
		}
	case strings.Contains(system, "ближайший к первоисточнику"):
		answer = w.primary
		if answer == "" {
			answer = `{"source": null}`
		}
	}
	fmt.Fprintf(rw, `{"choices":[{"message":{"content":%q}}]}`, answer)
}

const searxTwo = `{"results": [
 {"title": "AP", "url": "https://apnews.com/a", "content": "Democrat James Talarico has won the Texas Senate race against Ken Paxton.", "engines": ["yandex"]},
 {"title": "Blog", "url": "https://blog.example/b", "content": "Ken Paxton won the Texas Senate race, the blog claims.", "engines": ["yandex"]}
], "unresponsive_engines": []}`

const searxEmpty = `{"results": [], "unresponsive_engines": []}`

const braveOne = `{"web": {"results": [
 {"title": "NBC &amp; co", "url": "https://nbcnews.com/n", "description": "Talarico notched a Democratic Senate victory in Texas.", "extra_snippets": ["James Talarico defeated Ken Paxton on Tuesday night."], "page_age": "2026-11-04T05:00:00"}
]}}`

const claimText = "Talarico defeated Paxton in Texas."

// run wires the fakes and runs one check. braveQuota nil means Brave is off.
func run(t *testing.T, w *factWorld, braveQuota func() bool) (*FactCheck, error) {
	t.Helper()

	searx := httptest.NewServer(http.HandlerFunc(w.searx))
	t.Cleanup(searx.Close)
	brave := httptest.NewServer(http.HandlerFunc(w.brave))
	t.Cleanup(brave.Close)
	llm := httptest.NewServer(http.HandlerFunc(w.llm))
	t.Cleanup(llm.Close)

	t.Setenv("FACTCHECK_SEARCH_URL", searx.URL+"/")
	t.Setenv("AI_LLM_URL", llm.URL)
	t.Setenv("BRAVE_SEARCH_URL", brave.URL)
	t.Setenv("BRAVE_SEARCH_KEY", "test-key")
	saved := BraveQuota
	BraveQuota = braveQuota
	t.Cleanup(func() { BraveQuota = saved })
	noPacing(t)

	return checkFact(claimText, time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC))
}

func allow() bool { return true }

// noPacing swaps the shared search queue for one without pauses, so the tests
// do not wait ten seconds between searches.
func noPacing(t *testing.T) {
	t.Helper()
	saved := searchTurns
	searchTurns = newSearchPacer(0, 0, time.Minute)
	searchTurns.sleep = func(time.Duration) {}
	t.Cleanup(func() { searchTurns = saved })
}

func TestFactCheckConfirmsOnlyOnARealQuote(t *testing.T) {
	w := &factWorld{searxBody: searxTwo,
		evidence: []string{`{"evidence": [{"source": 1, "quote": "James Talarico has won the Texas Senate race", "relation": "supports"}]}`},
		primary:  `{"source": 1, "why": "agency"}`}
	result, err := run(t, w, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != FactConfirmed || len(result.Evidence) != 1 || result.Evidence[0].URL != "https://apnews.com/a" {
		t.Fatalf("got %+v", result)
	}
	if result.Primary == nil || result.Primary.URL != "https://apnews.com/a" {
		t.Errorf("primary: %+v", result.Primary)
	}
	if w.rechecks != 0 || w.braveCalls != 0 {
		t.Errorf("support needs no recheck, and Brave is off: rechecks %d, brave %d", w.rechecks, w.braveCalls)
	}
}

func TestFactCheckDropsInventedAndMisplacedQuotes(t *testing.T) {
	for name, evidence := range map[string]string{
		"invented":     `{"evidence": [{"source": 1, "quote": "Talarico won by a landslide of twenty points", "relation": "supports"}]}`,
		"wrong source": `{"evidence": [{"source": 2, "quote": "James Talarico has won the Texas Senate race", "relation": "supports"}]}`,
		"empty object": `{}`,
	} {
		w := &factWorld{searxBody: searxTwo, evidence: []string{evidence, evidence}}
		result, err := run(t, w, nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if result.Verdict != FactNotFound || len(result.Evidence) != 0 || result.Primary != nil {
			t.Errorf("%s: a quote that is not in its source must not count: %+v", name, result)
		}
	}
}

func TestFactCheckRefutesOnlyWhenTheRecheckAgrees(t *testing.T) {
	evidence := `{"evidence": [{"source": "2", "quote": "Ken Paxton won the Texas Senate race", "relation": "contradicts"}]}`
	cases := map[string]FactVerdict{"false": FactRefuted, "true": FactNotFound, `"maybe"`: FactNotFound}
	for answer, want := range cases {
		w := &factWorld{searxBody: searxTwo, evidence: []string{evidence, evidence}, bothCanBe: answer}
		result, err := run(t, w, nil)
		if err != nil {
			t.Fatal(err)
		}
		if result.Verdict != want {
			t.Errorf("recheck %s: got %s, want %s", answer, result.Verdict, want)
		}
	}
}

func TestFactCheckConflictingWhenBothSidesHold(t *testing.T) {
	w := &factWorld{searxBody: searxTwo, bothCanBe: "false", evidence: []string{`{"evidence": [
		{"source": 1, "quote": "James Talarico has won the Texas Senate race", "relation": "supports"},
		{"source": 2.0, "quote": "Ken Paxton won the Texas Senate race", "relation": "contradicts"}]}`}}
	result, err := run(t, w, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != FactConflicting || len(result.Evidence) != 2 {
		t.Fatalf("got %+v", result)
	}
}

func TestFactCheckNotCheckableSearchesNothing(t *testing.T) {
	w := &factWorld{plan: `{"checkable": false, "fresh": false, "queries": []}`, searxBody: searxTwo}
	result, err := run(t, w, allow)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != FactNotCheckable || w.searxCalls != 0 || w.braveCalls != 0 || w.evidenceCalls != 0 {
		t.Fatalf("got %+v, searx %d, brave %d, evidence %d", result, w.searxCalls, w.braveCalls, w.evidenceCalls)
	}
}

func TestFactCheckUnreadablePlanStillSearches(t *testing.T) {
	w := &factWorld{plan: `I think this is fine`, searxBody: searxTwo,
		evidence: []string{`{"evidence": [{"source": 1, "quote": "James Talarico has won the Texas Senate race", "relation": "supports"}]}`}}
	result, err := run(t, w, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != FactConfirmed || w.searxCalls != 1 {
		t.Fatalf("an unreadable plan must fall back to searching: %+v, searches %d", result, w.searxCalls)
	}
}

func TestFactCheckJudgesTheMessageAsWritten(t *testing.T) {
	// Even if the plan answer carries a "fixed" claim, the evidence question
	// gets the message word for word.
	w := &factWorld{plan: `{"checkable": true, "claim": "Paxton defeated Talarico", "queries": ["texas senate"]}`, searxBody: searxTwo}
	if _, err := run(t, w, nil); err != nil {
		t.Fatal(err)
	}
	if len(w.evidenceUsers) == 0 || !strings.Contains(w.evidenceUsers[0], "Утверждение: "+claimText) || strings.Contains(w.evidenceUsers[0], "Paxton defeated") {
		t.Fatalf("evidence question: %q", w.evidenceUsers)
	}
}

func TestFactCheckAsksBothEnginesAndMergesThem(t *testing.T) {
	w := &factWorld{plan: `{"checkable": true, "fresh": true, "queries": ["Таларико Пакстон", "talarico paxton"]}`,
		searxBody: searxTwo, braveBody: braveOne,
		evidence: []string{`{"evidence": [{"source": 2, "quote": "James Talarico defeated Ken Paxton on Tuesday night", "relation": "supports"}]}`}}
	result, err := run(t, w, allow)
	if err != nil {
		t.Fatal(err)
	}
	if w.searxCalls != 2 || w.braveCalls != 2 {
		t.Fatalf("every query goes to both engines: searx %d, brave %d", w.searxCalls, w.braveCalls)
	}
	// Merged by turns, Yandex first: [1] AP (yandex), [2] NBC (brave), [3] Blog.
	if result.Verdict != FactConfirmed || result.Evidence[0].URL != "https://nbcnews.com/n" || result.Evidence[0].Title != "NBC & co" {
		t.Fatalf("got %+v", result)
	}
	if !strings.Contains(w.braveQuery[0], "freshness=pm") || !strings.Contains(w.braveQuery[0], "search_lang=ru") || strings.Contains(w.braveQuery[1], "search_lang") {
		t.Errorf("brave params: %v", w.braveQuery)
	}
	if !strings.Contains(w.evidenceUsers[0], "Yandex — 4 результатов") || !strings.Contains(w.evidenceUsers[0], "Brave — 2 результатов") {
		t.Errorf("state line: %q", w.evidenceUsers[0])
	}
}

func TestFactCheckYandexDownBraveCarriesOn(t *testing.T) {
	w := &factWorld{searxCode: http.StatusBadGateway, braveBody: braveOne,
		evidence: []string{`{"evidence": [{"source": 1, "quote": "James Talarico defeated Ken Paxton on Tuesday night", "relation": "supports"}]}`}}
	result, err := run(t, w, allow)
	if err != nil {
		t.Fatalf("one engine down must not stop the check: %v", err)
	}
	if result.Verdict != FactConfirmed {
		t.Fatalf("got %+v", result)
	}
	if !strings.Contains(w.evidenceUsers[0], "Yandex — недоступен") {
		t.Errorf("the model must be told Yandex could not be asked: %q", w.evidenceUsers[0])
	}
}

func TestFactCheckBraveBudget(t *testing.T) {
	w := &factWorld{searxBody: searxTwo, braveBody: braveOne}
	if _, err := run(t, w, func() bool { return false }); err != nil {
		t.Fatal(err)
	}
	if w.braveCalls != 0 {
		t.Errorf("over the monthly budget Brave must not be called: %d", w.braveCalls)
	}

	w = &factWorld{searxBody: searxTwo, braveBody: braveOne}
	if _, err := run(t, w, nil); err != nil {
		t.Fatal(err)
	}
	if w.braveCalls != 0 {
		t.Errorf("without the budget hook Brave must not be called: %d", w.braveCalls)
	}

	t.Setenv("BRAVE_SEARCH_KEY", "")
	saved := BraveQuota
	BraveQuota = allow
	t.Cleanup(func() { BraveQuota = saved })
	if braveAllowed() {
		t.Error("without a key Brave must not be used")
	}
}

func TestBraveMonthlyLimit(t *testing.T) {
	for value, want := range map[string]int{"": 1900, "abc": 1900, "-5": 1900, "0": 0, " 1500 ": 1500} {
		t.Setenv("BRAVE_MONTHLY_LIMIT", value)
		if got := BraveMonthlyLimit(); got != want {
			t.Errorf("BRAVE_MONTHLY_LIMIT=%q: got %d, want %d", value, got, want)
		}
	}
}

func TestFactCheckSearchFailures(t *testing.T) {
	cases := []struct {
		name  string
		world *factWorld
		quota func() bool
		want  error
	}{
		{"our server down, Brave off", &factWorld{searxCode: http.StatusBadGateway}, nil, ErrSearchUnavailable},
		{"garbage from our server", &factWorld{searxBody: "<html>oops</html>"}, nil, ErrSearchUnavailable},
		{"engines refuse, Brave off", &factWorld{searxBody: `{"results": [], "unresponsive_engines": [["yandex", "CAPTCHA"]]}`}, nil, ErrSearchEnginesDown},
		{"engines refuse, Brave too", &factWorld{searxBody: `{"results": [], "unresponsive_engines": [["yandex", "timeout"]]}`, braveCode: http.StatusTooManyRequests}, allow, ErrSearchEnginesDown},
		{"our server down, Brave refuses", &factWorld{searxCode: http.StatusBadGateway, braveCode: http.StatusInternalServerError}, allow, ErrSearchEnginesDown},
	}
	for _, c := range cases {
		_, err := run(t, c.world, c.quota)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: want %v, got %v", c.name, c.want, err)
		}
		if c.world.evidenceCalls != 0 {
			t.Errorf("%s: nothing was searched, the model must not be asked", c.name)
		}
	}
}

func TestFactCheckNothingFoundIsNotFound(t *testing.T) {
	w := &factWorld{searxBody: searxEmpty, braveBody: `{"web": {"results": []}}`}
	result, err := run(t, w, allow)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != FactNotFound || w.evidenceCalls != 0 {
		t.Fatalf("engines answered with nothing: not found, without asking the model: %+v", result)
	}
}

func TestFactCheckSecondRound(t *testing.T) {
	quote := `{"evidence": [{"source": 1, "quote": "James Talarico has won the Texas Senate race", "relation": "supports"}]}`
	w := &factWorld{
		searxBodies: []string{`{"results": [{"title": "Other", "url": "https://x.example/o", "content": "Something about Texas weather today and tomorrow.", "engines": ["yandex"]}], "unresponsive_engines": []}`, searxTwo},
		evidence:    []string{`{"evidence": []}`, quote},
		refine:      `{"query": "Talarico Paxton result"}`,
	}
	result, err := run(t, w, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != FactConfirmed || w.searxCalls != 2 || w.refines != 1 || w.evidenceCalls != 2 {
		t.Fatalf("got %+v; searches %d, refines %d, evidence %d", result, w.searxCalls, w.refines, w.evidenceCalls)
	}

	// A refined query that repeats one already made is not searched again.
	w = &factWorld{searxBody: searxTwo, refine: `{"query": "TALARICO PAXTON"}`}
	if _, err := run(t, w, nil); err != nil {
		t.Fatal(err)
	}
	if w.searxCalls != 1 {
		t.Errorf("a repeated query must not be searched again: %d searches", w.searxCalls)
	}
}

func TestFactCheckPrimaryMustBeInTheList(t *testing.T) {
	quote := `{"evidence": [{"source": 1, "quote": "James Talarico has won the Texas Senate race", "relation": "supports"}]}`
	for answer, want := range map[string]string{
		`{"source": 2}`:    "https://blog.example/b",
		`{"source": 9}`:    "",
		`{"source": null}`: "",
		`nonsense`:         "",
	} {
		w := &factWorld{searxBody: searxTwo, evidence: []string{quote}, primary: answer}
		result, err := run(t, w, nil)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		if result.Primary != nil {
			got = result.Primary.URL
		}
		if got != want {
			t.Errorf("primary answer %s: got %q, want %q", answer, got, want)
		}
	}
}

func TestMergeResultsAlternatesAndDedupes(t *testing.T) {
	r := func(url, engine string) searchResult {
		return searchResult{URL: url, Content: "text", Engines: []string{engine}}
	}
	pools := searchPools{
		yandex: []searchResult{r("a", "yandex"), r("b", "yandex"), r("c", "yandex"), {URL: "empty", Engines: []string{"yandex"}}},
		brave:  []searchResult{r("b", "brave"), r("d", "brave")},
	}
	seen := map[string]bool{}
	got := []string{}
	for _, x := range mergeResults(pools, seen, 10) {
		got = append(got, x.URL)
	}
	if strings.Join(got, ",") != "a,b,d,c" {
		t.Errorf("merged %v, want a,b,d,c", got)
	}
	if again := mergeResults(pools, seen, 10); len(again) != 0 {
		t.Errorf("seen urls must not come back in a later round: %v", again)
	}
}

func TestSearchFailureClassification(t *testing.T) {
	ok := engineState{used: true, results: 3}
	nothing := engineState{used: true}
	ourDown := engineState{used: true, failures: []string{"unreachable"}, ourSide: 1}
	refused := engineState{used: true, failures: []string{"CAPTCHA"}}
	unused := engineState{}
	cases := []struct {
		state searchState
		want  error
	}{
		{searchState{yandex: ok, brave: refused}, nil},
		{searchState{yandex: nothing, brave: unused}, nil},
		{searchState{yandex: ourDown, brave: unused}, ErrSearchUnavailable},
		{searchState{yandex: refused, brave: unused}, ErrSearchEnginesDown},
		{searchState{yandex: ourDown, brave: refused}, ErrSearchEnginesDown},
		{searchState{}, ErrSearchUnavailable},
	}
	for i, c := range cases {
		err := searchFailure(c.state)
		if (c.want == nil) != (err == nil) || (c.want != nil && !errors.Is(err, c.want)) {
			t.Errorf("case %d: got %v, want %v", i, err, c.want)
		}
	}
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

	for i := 0; i < 3; i++ {
		if err := pacer.wait(); err != nil {
			t.Fatal(err)
		}
	}
	if len(clock.slept) != 2 || clock.slept[0] != 10*time.Second || clock.slept[1] != 20*time.Second {
		t.Fatalf("slept %v, want [10s 20s]", clock.slept)
	}

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
	if err := pacer.wait(); !errors.Is(err, ErrSearchBusy) {
		t.Fatalf("want ErrSearchBusy, got %v", err)
	}
	clock.at = clock.at.Add(10 * time.Second)
	if err := pacer.wait(); err != nil {
		t.Fatalf("after the queue moved: %v", err)
	}
}

func TestSearchPacerWaitAtMost(t *testing.T) {
	clock := &fakeClock{at: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)}
	pacer := newSearchPacer(30*time.Second, 2*time.Minute, 5*time.Minute)
	pacer.now, pacer.sleep = clock.now, clock.sleep

	if err := pacer.waitAtMost(15 * time.Second); err != nil {
		t.Fatalf("a free turn: %v", err)
	}
	// The next turn is 30 s away: too long for 15 s of patience, and nothing
	// is booked, so the patient waiter after it is not pushed further.
	if err := pacer.waitAtMost(15 * time.Second); !errors.Is(err, ErrSearchBusy) {
		t.Fatalf("want ErrSearchBusy, got %v", err)
	}
	if err := pacer.wait(); err != nil || len(clock.slept) != 1 || clock.slept[0] != 30*time.Second {
		t.Fatalf("the patient one waits its 30 s: %v %v", err, clock.slept)
	}
}

func TestFactCheckSkipsAQueuedYandexWhenBraveIsThere(t *testing.T) {
	w := &factWorld{searxBody: searxTwo, braveBody: braveOne,
		plan: `{"checkable": true, "queries": ["one", "two"]}`}
	searx := httptest.NewServer(http.HandlerFunc(w.searx))
	t.Cleanup(searx.Close)
	brave := httptest.NewServer(http.HandlerFunc(w.brave))
	t.Cleanup(brave.Close)
	llm := httptest.NewServer(http.HandlerFunc(w.llm))
	t.Cleanup(llm.Close)
	t.Setenv("FACTCHECK_SEARCH_URL", searx.URL)
	t.Setenv("AI_LLM_URL", llm.URL)
	t.Setenv("BRAVE_SEARCH_URL", brave.URL)
	t.Setenv("BRAVE_SEARCH_KEY", "test-key")
	saved, savedTurns := BraveQuota, searchTurns
	t.Cleanup(func() { BraveQuota, searchTurns = saved, savedTurns })
	BraveQuota = allow

	// Yandex is under pressure: a minute between searches.
	clock := &fakeClock{at: time.Now()}
	searchTurns = newSearchPacer(time.Minute, 2*time.Minute, 5*time.Minute)
	searchTurns.now, searchTurns.sleep = clock.now, clock.sleep

	if _, err := checkFact(claimText, time.Now()); err != nil {
		t.Fatal(err)
	}
	if w.searxCalls != 1 || w.braveCalls != 2 || len(clock.slept) != 0 {
		t.Fatalf("the second Yandex search must be skipped, not waited for: searx %d, brave %d, slept %v", w.searxCalls, w.braveCalls, clock.slept)
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

	pacer.observe(found("yandex", "yandex"), ddg, false)
	if pacer.interval != 20*time.Second {
		t.Fatalf("first captcha: interval %s, want 20s", pacer.interval)
	}
	pacer.observe(found("yandex"), ddg, false)
	if pacer.interval != 10*time.Second {
		t.Fatalf("a chronic captcha must not keep the interval up: %s", pacer.interval)
	}

	pacer.next = clock.at
	pacer.observe(found("bing news"), [][]string{{"duckduckgo", "CAPTCHA"}, {"yandex", "timeout"}}, false)
	if pacer.interval != 20*time.Second || !pacer.next.Equal(clock.at.Add(20*time.Second)) {
		t.Fatalf("yandex failing: interval %s next %s", pacer.interval, pacer.next.Sub(clock.at))
	}
	for i := 0; i < 6; i++ {
		pacer.observe(nil, [][]string{{"duckduckgo", "CAPTCHA"}, {"yandex", "CAPTCHA"}}, true)
	}
	if pacer.interval != 2*time.Minute {
		t.Fatalf("cap: %s", pacer.interval)
	}
	for i, w := range []time.Duration{time.Minute, 30 * time.Second, 15 * time.Second, 10 * time.Second} {
		pacer.observe(found("yandex"), ddg, false)
		if pacer.interval != w {
			t.Fatalf("recovery step %d: %s, want %s", i+1, pacer.interval, w)
		}
	}
}

func TestTodayLineIsMoscowDate(t *testing.T) {
	got := todayLine(time.Date(2026, 11, 5, 22, 30, 0, 0, time.UTC))
	if got != "Сегодня 6 ноября 2026 года. " {
		t.Errorf("todayLine = %q", got)
	}
}

func TestNormalizedMatchesAcrossPunctuation(t *testing.T) {
	snippet := normalized(snippetOf(searchResult{Title: "AP", Content: "Democrat James Talarico has won, the Texas Senate race!", Engines: []string{"brave"}, PublishedDate: "2026-11-03T10:00"}))
	if !strings.Contains(snippet, normalized("James Talarico has won the Texas  Senate race")) {
		t.Error("spacing and punctuation must not matter")
	}
	if !strings.HasPrefix(snippet, "brave 2026 11 03") {
		t.Errorf("the snippet must name its engine and date: %q", snippet)
	}
}
