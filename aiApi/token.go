package aiApi

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

// ErrUnauthorized means our generation service refused the request because the
// token is missing or does not match. It is told apart from other failures on
// purpose: the service is perfectly healthy, and no number of retries will help
// — the bot's AI_API_TOKEN simply differs from the one on the host.
var ErrUnauthorized = errors.New("the generation service rejected our token")

// tokenHeader is the service's own header. Authorization: Bearer works as well,
// but a dedicated header leaves Authorization free for whatever may sit in front
// of the service one day.
//
// The token never travels in the query string. It would end up in the proxy
// logs, in the browser history and in the Referer of everything the service
// serves, and the service rejects that form anyway.
const tokenHeader = "X-API-Token"

var tokenWarning sync.Once

// apiToken reads the token per request instead of caching it at startup: main()
// loads .env.local through godotenv, and package-level initialisation runs
// BEFORE main, so a value captured there would always be empty.
//
// Spaces are trimmed because the value is pasted into .env.local by hand, and a
// trailing one is invisible but breaks the comparison on the other side.
func apiToken() string {
	return strings.TrimSpace(os.Getenv("AI_API_TOKEN"))
}

// tokenTransport signs every request made by the clients that talk to our own
// generation service.
//
// It lives in the transport rather than at the call sites deliberately. Eight
// places build such a request today, and a token forgotten in one of them would
// not break the build: it would come back as a 401 in production, on one kind of
// job only, long after the change. This way a call site added tomorrow is signed
// by default — and the LLM client, which goes to another host entirely and has
// no business carrying our token, is left untouched.
type tokenTransport struct {
	next http.RoundTripper
}

func (t tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token := apiToken()
	if token == "" {
		// Not fatal, and that is the point: while the token is being rolled out
		// the service still accepts unsigned requests, so the bot can be
		// taught the header before the host starts demanding it.
		tokenWarning.Do(func() {
			log.Println("[warn] AI_API_TOKEN is empty, requests to the generation service go unsigned")
		})

		return t.next.RoundTrip(req)
	}

	// RoundTrip must not modify the request it was handed, so the header goes
	// onto a copy.
	signed := req.Clone(req.Context())
	signed.Header.Set(tokenHeader, token)

	return t.next.RoundTrip(signed)
}

// apiTransport wraps the shared transport, so the clients keep using the same
// connection pool they used before the token.
func apiTransport() http.RoundTripper {
	return tokenTransport{next: http.DefaultTransport}
}
