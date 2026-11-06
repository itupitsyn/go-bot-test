package aiApi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJokeProblem(t *testing.T) {
	recent := map[string]bool{jokeOpener("Проверил зеркало: да."): true}
	cases := map[string]string{
		"Лапки подтверждены. Носик не проверял.": "",
		"Проверил холодильник: йогурта нет.":     "same opening as a recent joke",
		"ок": "empty",
		"Официально подтверждаю, понедельник официально тяжёлый.": "repeated word",
		strings.Repeat("слово ", 19): "too long",
		`{"jokes": "x y"}`:           "garbage",
		"Мнения не проверяю, у них иммунитет.": "copy of an example",
	}
	for joke, want := range cases {
		if got := jokeProblem(joke, recent); got != want {
			t.Errorf("jokeProblem(%q) = %q, want %q", joke, got, want)
		}
	}
}

func TestJokeOpenerIsTheFirstStem(t *testing.T) {
	if jokeOpener("«Проверил» всех.") != jokeOpener("проверила кота") {
		t.Error("case, quotes and endings must not matter")
	}
}

// jokeLLM answers the generation with jokes and the pick with best.
func jokeLLM(t *testing.T, jokes string, best string, prompts *[]string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		json.Unmarshal(body, &request)
		system, user := request.Messages[0].Content, request.Messages[1].Content
		*prompts = append(*prompts, system+"\n---\n"+user)
		answer := jokes
		if strings.Contains(system, "Выбери") {
			answer = best
		}
		fmt.Fprintf(rw, `{"choices":[{"message":{"content":%q}}]}`, answer)
	}))
	t.Cleanup(server.Close)
	t.Setenv("AI_LLM_URL", server.URL)
}

func TestGenerateFactJokePicks(t *testing.T) {
	var prompts []string
	jokeLLM(t, `{"jokes": ["Лапки подтверждены. Носик не проверял.", "Проверил: лапки настоящие.", "Экспертиза: лапки милые."]}`, `{"best": 3}`, &prompts)

	joke, err := generateFactJoke("у меня лапки", []string{"Проверил зеркало: да."})
	if err != nil {
		t.Fatal(err)
	}
	// "Проверил..." is dropped for repeating a recent opening, so two are left;
	// the model's "3" is out of range for them, and the first one stands.
	if joke != "Лапки подтверждены. Носик не проверял." {
		t.Fatalf("got %q", joke)
	}
	if len(prompts) != 2 || !strings.Contains(prompts[0], "недавно уже ответил так:\n- Проверил зеркало: да.") {
		t.Fatalf("the recent jokes must reach the model: %q", prompts)
	}
	if strings.Contains(prompts[1], "Проверил: лапки настоящие.") {
		t.Error("a dropped joke must not be offered for the pick")
	}
}

func TestGenerateFactJokeFallbacks(t *testing.T) {
	var prompts []string
	jokeLLM(t, `{"jokes": ["Проверил: лапки настоящие."]}`, `{"best": 1}`, &prompts)
	joke, _ := generateFactJoke("у меня лапки", []string{"Проверил зеркало: да."})
	if joke != "Проверил: лапки настоящие." {
		t.Errorf("a repeated opening beats no joke at all: %q", joke)
	}

	prompts = nil
	jokeLLM(t, `not json`, `{"best": 1}`, &prompts)
	if joke, err := generateFactJoke("у меня лапки", nil); joke != "" || err != nil {
		t.Errorf("an unreadable answer gives no joke and no error: %q %v", joke, err)
	}
}
