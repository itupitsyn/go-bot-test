package aiApi

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Two search engines are asked at once and the model reads both: Yandex through
// our SearXNG on the VPS, and Brave through its API. Measured 06.11.2026 they are
// about equally good (69-70 of 73 each on the same chain), and they fail at
// different times: in the final run Yandex timed out on one search in eight, and
// Brave covered every one of those.

const (
	// searchTimeout covers one request to our search server. It waits for the
	// slowest engine itself (ten seconds at most in its settings), so this is
	// only for a server that hangs.
	searchTimeout = 30 * time.Second
	// braveTimeout covers one request to the Brave API.
	braveTimeout = 20 * time.Second
	// maxSnippets is how many results the model gets, both engines together.
	maxSnippets = 10
	// braveDefaultEndpoint is overridable for tests only.
	braveDefaultEndpoint = "https://api.search.brave.com/res/v1/web/search"
	// braveDefaultMonthlyLimit stays below the 2000 requests a month the owner
	// pays for ($5 credit + $5), for the requests our own tests and retries eat.
	braveDefaultMonthlyLimit = 1900
)

var (
	searchClient = &http.Client{Timeout: searchTimeout}
	braveClient  = &http.Client{Timeout: braveTimeout}
)

// BraveQuota takes one request out of the monthly Brave budget and says whether
// it may go. It is wired to the database in main; left nil, Brave is not used
// at all, so a missing hook can never mean an unbounded bill.
var BraveQuota func() bool

// BraveMonthlyLimit is the budget BraveQuota should enforce: BRAVE_MONTHLY_LIMIT,
// or the default.
func BraveMonthlyLimit() int {
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("BRAVE_MONTHLY_LIMIT"))); err == nil && value >= 0 {
		return value
	}

	return braveDefaultMonthlyLimit
}

const (
	// searchInterval is the least time between two searches on our SearXNG from
	// the whole bot, whoever asks, when the engines are calm. Engines answer a
	// burst with a captcha, and a captcha then hurts every chat. 10 s is a
	// guess: what was measured is that at one search per 30 s Yandex never
	// refused (73 of 73, 05.11.2026). The guess does not have to be right: see
	// observe. Brave is a paid API and needs no pacing.
	searchInterval = 10 * time.Second
	// searchMaxInterval caps how far the interval grows under pressure.
	searchMaxInterval = 2 * time.Minute
	// searchMaxWait is how long a search may wait for its turn. Beyond it that
	// search is skipped, and the check goes on with Brave alone.
	searchMaxWait = 5 * time.Minute
	// yandexSkipWait is how long a Yandex search waits when Brave is in the same
	// check. A check makes 2-3 searches; with the interval grown under pressure,
	// waiting each turn made checks of two minutes and more (06.11.2026) while
	// Brave had long answered. Skipping also eases the pressure that grew it.
	yandexSkipWait = 15 * time.Second
)

// searchPacer hands out turns to the search, at least interval apart, in the
// order they were asked for. A turn is booked under the lock and waited for
// outside it, so a long queue never holds the lock.
//
// The interval adjusts itself: it doubles when the engines show pressure and
// halves back toward base after each calm search.
type searchPacer struct {
	mu       sync.Mutex
	next     time.Time
	interval time.Duration
	base     time.Duration
	max      time.Duration
	maxWait  time.Duration
	now      func() time.Time
	sleep    func(time.Duration)
	// failing remembers, per engine, whether it failed the last time it was
	// heard from. Pressure is an engine that worked and now fails, not one
	// that fails all the time: DuckDuckGo sits in a captcha for days on this
	// address, and counting that would pin the interval to the cap for good.
	failing map[string]bool
}

func newSearchPacer(base, max, maxWait time.Duration) *searchPacer {
	return &searchPacer{interval: base, base: base, max: max, maxWait: maxWait,
		now: time.Now, sleep: time.Sleep, failing: map[string]bool{}}
}

var searchTurns = newSearchPacer(searchInterval, searchMaxInterval, searchMaxWait)

// observe reads one search's outcome and adjusts the interval. allDown is a
// search in which every engine failed: pressure whatever the engines' history.
// Our own server being unreachable is not the engines' doing and is not
// reported here.
func (p *searchPacer) observe(results []searchResult, failed [][]string, allDown bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	pressure := allDown
	newlyFailing := []string{}
	failedNow := map[string]bool{}
	for _, item := range failed {
		if len(item) == 0 {
			continue
		}
		failedNow[item[0]] = true
		if wasFailing, known := p.failing[item[0]]; !known || !wasFailing {
			// An engine never heard from before counts as working: a fresh
			// start must not hide the first captcha.
			pressure = true
			newlyFailing = append(newlyFailing, strings.Join(item, ": "))
		}
	}
	for engine := range failedNow {
		p.failing[engine] = true
	}
	for engine := range engineCounts(results) {
		p.failing[engine] = false
	}

	before := p.interval
	if pressure {
		p.interval *= 2
		if p.interval > p.max {
			p.interval = p.max
		}
		// The turn already booked was spaced by the old interval: push it.
		if next := p.now().Add(p.interval); next.After(p.next) {
			p.next = next
		}
	} else {
		p.interval /= 2
		if p.interval < p.base {
			p.interval = p.base
		}
	}
	if p.interval != before {
		log.Printf("search pace: interval %s -> %s (all engines down: %v, newly failing: %v)", before, p.interval, allDown, newlyFailing)
	}
}

// wait books the next free turn and sleeps until it comes. If that turn is
// further away than maxWait, nothing is booked and ErrSearchBusy comes back.
func (p *searchPacer) wait() error {
	return p.waitAtMost(p.maxWait)
}

// waitAtMost is wait with a shorter patience of its own.
func (p *searchPacer) waitAtMost(limit time.Duration) error {
	if limit > p.maxWait {
		limit = p.maxWait
	}
	p.mu.Lock()
	now := p.now()
	turn := p.next
	if turn.Before(now) {
		turn = now
	}
	if turn.Sub(now) > limit {
		p.mu.Unlock()
		return fmt.Errorf("%w: next turn in %s", ErrSearchBusy, turn.Sub(now).Round(time.Second))
	}
	p.next = turn.Add(p.interval)
	p.mu.Unlock()

	if delay := turn.Sub(now); delay > 0 {
		p.sleep(delay)
	}

	return nil
}

// searchResult is one result, in the shape SearXNG answers with; Brave results
// are put into the same shape.
type searchResult struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	Content       string   `json:"content"`
	PublishedDate string   `json:"publishedDate"`
	Engines       []string `json:"engines"`
}

type searchResponse struct {
	Results []searchResult `json:"results"`
	// Each item is [engine, reason]. Kept raw: the reason is only logged.
	Unresponsive [][]string `json:"unresponsive_engines"`
}

// searchSearx asks our SearXNG. Zero results with every engine failed is
// ErrSearchEnginesDown; zero results with the engines fine is just nothing
// found and comes back as an empty slice. The engines that failed come back
// too, even when others answered: only the log and the pacer need them.
func searchSearx(query string) ([]searchResult, [][]string, error) {
	base := strings.TrimRight(os.Getenv("FACTCHECK_SEARCH_URL"), "/")
	if base == "" {
		return nil, nil, fmt.Errorf("%w: FACTCHECK_SEARCH_URL is not set", ErrSearchUnavailable)
	}

	address := base + "/search?" + url.Values{"q": {query}, "format": {"json"}}.Encode()
	res, err := searchClient.Get(address)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrSearchUnavailable, err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrSearchUnavailable, err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("%w: status %d", ErrSearchUnavailable, res.StatusCode)
	}

	var parsed searchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, nil, fmt.Errorf("%w: bad json: %v", ErrSearchUnavailable, err)
	}

	if len(parsed.Results) == 0 && len(parsed.Unresponsive) > 0 {
		return nil, parsed.Unresponsive, fmt.Errorf("%w: %v", ErrSearchEnginesDown, parsed.Unresponsive)
	}

	return parsed.Results, parsed.Unresponsive, nil
}

var cyrillic = regexp.MustCompile(`[а-яёА-ЯЁ]`)

type braveResponse struct {
	Web struct {
		Results []struct {
			Title         string   `json:"title"`
			URL           string   `json:"url"`
			Description   string   `json:"description"`
			ExtraSnippets []string `json:"extra_snippets"`
			PageAge       string   `json:"page_age"`
		} `json:"results"`
	} `json:"web"`
}

// searchBrave asks the Brave API. Its "extra snippets", up to five more
// excerpts of a page, go into the content: more text for a deciding phrase to
// be in. fresh narrows the search to the last month.
func searchBrave(query string, fresh bool) ([]searchResult, error) {
	key := strings.TrimSpace(os.Getenv("BRAVE_SEARCH_KEY"))
	endpoint := strings.TrimSpace(os.Getenv("BRAVE_SEARCH_URL"))
	if endpoint == "" {
		endpoint = braveDefaultEndpoint
	}

	params := url.Values{"q": {query}, "count": {"10"}, "extra_snippets": {"true"}, "text_decorations": {"false"}}
	if fresh {
		params.Set("freshness", "pm")
	}
	if cyrillic.MatchString(query) {
		params.Set("search_lang", "ru")
	}
	request, err := http.NewRequest(http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Subscription-Token", key)

	res, err := braveClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("too many requests")
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", res.StatusCode)
	}

	var parsed braveResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("bad json: %v", err)
	}

	results := make([]searchResult, 0, len(parsed.Web.Results))
	for _, item := range parsed.Web.Results {
		parts := []string{}
		for _, part := range append([]string{item.Description}, item.ExtraSnippets...) {
			if part = strings.TrimSpace(part); part != "" {
				parts = append(parts, part)
			}
		}
		results = append(results, searchResult{
			Title:         html.UnescapeString(strings.TrimSpace(item.Title)),
			URL:           item.URL,
			Content:       html.UnescapeString(strings.Join(parts, " … ")),
			PublishedDate: item.PageAge,
			Engines:       []string{"brave"},
		})
	}

	return results, nil
}

// engineCounts says which engines the results came from, for the log.
func engineCounts(results []searchResult) map[string]int {
	counts := map[string]int{}
	for _, result := range results {
		for _, engine := range result.Engines {
			counts[engine]++
		}
	}

	return counts
}

// engineState is how one engine did over a check's searches. used is false for
// Brave when it was not asked at all (no key, no budget left): that is not a
// failure, and the model is not told about an engine that was never asked.
type engineState struct {
	used     bool
	results  int
	failures []string
	// ourSide counts failures that are ours, not the engine's: our SearXNG
	// unreachable, its queue too long. All failures ours means "the search is
	// down"; any engine-side failure means "the engines do not answer".
	ourSide int
}

// searchState is what both engines did during one check.
type searchState struct {
	yandex engineState
	brave  engineState
}

// searchPools holds each engine's results apart, so that neither crowds the
// other out when they are merged.
type searchPools struct {
	yandex []searchResult
	brave  []searchResult
}

// braveAllowed says whether this search may use Brave: there is a key, the
// quota hook is wired, and the month's budget has room.
func braveAllowed() bool {
	if strings.TrimSpace(os.Getenv("BRAVE_SEARCH_KEY")) == "" || BraveQuota == nil {
		return false
	}
	if !BraveQuota() {
		log.Println("fact check: Brave monthly limit reached, searching with Yandex only")
		return false
	}

	return true
}

// searchBoth runs every query on both engines at once: Brave in its own
// goroutine, Yandex in the shared paced queue.
func searchBoth(queries []string, fresh bool, state *searchState) searchPools {
	var pools searchPools
	for _, query := range queries {
		var braveResults []searchResult
		var braveErr error
		var wg sync.WaitGroup
		braveUsed := braveAllowed()
		if braveUsed {
			wg.Add(1)
			go func() {
				defer wg.Done()
				braveResults, braveErr = searchBrave(query, fresh)
			}()
		}

		state.yandex.used = true
		patience := searchMaxWait
		if braveUsed {
			patience = yandexSkipWait
		}
		if err := searchTurns.waitAtMost(patience); err != nil {
			state.yandex.failures = append(state.yandex.failures, err.Error())
			state.yandex.ourSide++
		} else {
			found, failed, err := searchSearx(query)
			if err == nil || errorsIsEnginesDown(err) {
				searchTurns.observe(found, failed, err != nil)
			}
			switch {
			case err == nil:
				state.yandex.results += len(found)
				pools.yandex = append(pools.yandex, found...)
			case errorsIsEnginesDown(err):
				state.yandex.failures = append(state.yandex.failures, err.Error())
			default:
				state.yandex.failures = append(state.yandex.failures, err.Error())
				state.yandex.ourSide++
			}
		}

		wg.Wait()
		if braveUsed {
			state.brave.used = true
			if braveErr != nil {
				state.brave.failures = append(state.brave.failures, braveErr.Error())
			} else {
				state.brave.results += len(braveResults)
				pools.brave = append(pools.brave, braveResults...)
			}
		}
	}

	return pools
}

// mergeResults takes results by turns, Yandex then Brave, skipping ones without
// text and urls already taken (seen is shared across rounds).
func mergeResults(pools searchPools, seen map[string]bool, limit int) []searchResult {
	usable := func(list []searchResult) []searchResult {
		kept := []searchResult{}
		for _, r := range list {
			if r.URL != "" && strings.TrimSpace(r.Content) != "" {
				kept = append(kept, r)
			}
		}
		return kept
	}
	lists := [][]searchResult{usable(pools.yandex), usable(pools.brave)}

	merged := []searchResult{}
	for index := 0; len(merged) < limit; index++ {
		progressed := false
		for _, list := range lists {
			if index >= len(list) || len(merged) == limit {
				continue
			}
			progressed = true
			if r := list[index]; !seen[r.URL] {
				seen[r.URL] = true
				merged = append(merged, r)
			}
		}
		if !progressed {
			break
		}
	}

	return merged
}

// stateLine tells the model how the search went, so that an engine that could
// not be asked is not read as an engine that found nothing.
func stateLine(state searchState) string {
	parts := []string{}
	for _, item := range []struct {
		title string
		e     engineState
	}{{"Yandex", state.yandex}, {"Brave", state.brave}} {
		switch {
		case !item.e.used:
			continue
		case item.e.results == 0 && len(item.e.failures) > 0:
			parts = append(parts, item.title+" — недоступен, его молчание ничего не говорит об утверждении")
		case item.e.results == 0:
			parts = append(parts, item.title+" — ответил, но ничего не нашёл")
		case len(item.e.failures) > 0:
			parts = append(parts, fmt.Sprintf("%s — %d результатов, часть запросов не прошла", item.title, item.e.results))
		default:
			parts = append(parts, fmt.Sprintf("%s — %d результатов", item.title, item.e.results))
		}
	}

	return "Состояние поиска: " + strings.Join(parts, "; ") + "."
}

// searchFailure says why nothing at all came back, or nil when something did
// or an engine answered with nothing. Our side only means the search is down;
// anything else means the engines would not answer.
func searchFailure(state searchState) error {
	engines := []engineState{state.yandex, state.brave}
	answered, used, ourSideOnly := false, false, true
	for _, e := range engines {
		if !e.used {
			continue
		}
		used = true
		if e.results > 0 || len(e.failures) == 0 {
			answered = true
		}
		if e.ourSide < len(e.failures) {
			ourSideOnly = false
		}
	}
	switch {
	case !used:
		return fmt.Errorf("%w: no engine configured", ErrSearchUnavailable)
	case answered:
		return nil
	case ourSideOnly:
		return fmt.Errorf("%w: yandex %v, brave %v", ErrSearchUnavailable, state.yandex.failures, state.brave.failures)
	default:
		return fmt.Errorf("%w: yandex %v, brave %v", ErrSearchEnginesDown, state.yandex.failures, state.brave.failures)
	}
}
