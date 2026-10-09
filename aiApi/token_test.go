package aiApi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// tokenEcho starts a stub that records the token header of every request, so a
// test can see exactly what reached the other side.
func tokenEcho(t *testing.T, body string) (string, *atomic.Value) {
	t.Helper()

	var seen atomic.Value
	seen.Store("")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.Header.Get(tokenHeader))
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return server.URL, &seen
}

func TestRequestsCarryTheToken(t *testing.T) {
	t.Setenv("AI_API_TOKEN", "t-123")
	host, seen := tokenEcho(t, `{"running": null, "pending": []}`)

	if _, err := getQueueStatus(host, "any"); err != nil {
		t.Fatal(err)
	}

	if got := seen.Load(); got != "t-123" {
		t.Fatalf("сервис получил токен %q вместо нашего", got)
	}
}

func TestEmptyTokenSendsNoHeader(t *testing.T) {
	// The rollout leans on this: the bot is taught the header before the host
	// starts demanding it, and until the value is set nothing must change.
	t.Setenv("AI_API_TOKEN", "")
	host, seen := tokenEcho(t, `{"running": null, "pending": []}`)

	if _, err := getQueueStatus(host, "any"); err != nil {
		t.Fatal(err)
	}

	if got := seen.Load(); got != "" {
		t.Fatalf("без токена отправился заголовок %q", got)
	}
}

func TestTokenIsTrimmed(t *testing.T) {
	// A value pasted into .env.local by hand collects invisible spaces, and the
	// service compares byte for byte.
	t.Setenv("AI_API_TOKEN", "  t-123\t")
	host, seen := tokenEcho(t, `{"running": null, "pending": []}`)

	if _, err := getQueueStatus(host, "any"); err != nil {
		t.Fatal(err)
	}

	if got := seen.Load(); got != "t-123" {
		t.Fatalf("пробелы не срезались: %q", got)
	}
}

// recordingRT stands in for the real transport and remembers what it was asked
// to send.
type recordingRT struct {
	got http.Header
}

func (r *recordingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	r.got = req.Header.Clone()

	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestOriginalRequestIsLeftAlone(t *testing.T) {
	// http.RoundTripper must not modify the request it is handed. Breaking that
	// shows up only in a retry, where the same request object is reused.
	t.Setenv("AI_API_TOKEN", "t-123")

	req, err := http.NewRequest("GET", "http://example.invalid/api/queue", nil)
	if err != nil {
		t.Fatal(err)
	}

	inner := &recordingRT{}
	if _, err := (tokenTransport{next: inner}).RoundTrip(req); err != nil {
		t.Fatal(err)
	}

	if got := inner.got.Get(tokenHeader); got != "t-123" {
		t.Fatalf("транспорт не подписал запрос: %q", got)
	}
	if got := req.Header.Get(tokenHeader); got != "" {
		t.Fatalf("исходный запрос изменён, в нём оказался заголовок %q", got)
	}
}

func TestRejectedTokenIsNotRetried(t *testing.T) {
	// Repeating a 401 only delays the plain answer and buries the reason under
	// "failed 3 times in a row".
	t.Setenv("AI_API_TOKEN", "wrong")

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"detail": "нужен токен"}`))
	}))
	t.Cleanup(server.Close)

	_, err := waitData("image", server.URL, "any", 10*time.Millisecond, time.Second, nil)

	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("401 не распознан как отказ по токену: %v", err)
	}
	if n := requests.Load(); n != 1 {
		t.Fatalf("запросов с заведомо неверным токеном: %d, ожидался один", n)
	}
}

func TestSubmitRejectionIsRecognised(t *testing.T) {
	err := checkSubmitStatus("image", http.StatusUnauthorized, []byte(`{"detail": "нужен токен"}`))

	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("отказ при постановке задачи не распознан: %v", err)
	}
	if errors.Is(err, ErrQueueFull) {
		t.Fatal("отказ по токену принят за переполненную очередь")
	}
}

func TestLLMClientCarriesNoToken(t *testing.T) {
	// The LLM lives on another host. Unifying the clients one day must not
	// quietly hand it a secret it never asked for.
	if llmClient.Transport != nil {
		t.Fatal("клиент LLM получил наш транспорт с токеном")
	}
}
