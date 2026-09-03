package aiApi

import "testing"

func TestContentOf(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			"обычный ответ",
			`{"choices":[{"message":{"content":"a cat on a windowsill"}}]}`,
			"a cat on a windowsill",
		},
		{
			"пробелы по краям срезаются",
			`{"choices":[{"message":{"content":"  a cat\n"}}]}`,
			"a cat",
		},
	}

	for _, c := range cases {
		got, err := contentOf([]byte(c.body))
		if err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: want %q, got %q", c.name, c.want, got)
		}
	}
}

// TestContentOfBadResponses — ровно те формы ответа, на которых прежняя цепочка
// приведений типа падала в панику и уносила процесс целиком. Здесь важно не
// то, какая именно вернётся ошибка, а что она вернётся.
func TestContentOfBadResponses(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"пустой choices", `{"choices":[]}`},
		{"choices нет вовсе", `{}`},
		{"choices не массив", `{"choices":"nope"}`},
		{"message не объект", `{"choices":[{"message":"nope"}]}`},
		{"content не строка", `{"choices":[{"message":{"content":42}}]}`},
		{"ошибка вместо ответа", `{"error":{"message":"context window exceeded"}}`},
		{"пустой content", `{"choices":[{"message":{"content":"   "}}]}`},
		{"не json", `<html>502 Bad Gateway</html>`},
		{"пустое тело", ``},
		{"json null", `null`},
	}

	for _, c := range cases {
		got, err := contentOf([]byte(c.body))
		if err == nil {
			t.Errorf("%s: want an error, got %q", c.name, got)
		}
	}
}

func TestApplyPromptTemplate(t *testing.T) {
	cases := [][3]string{
		// Точка в конце перевода не должна удваиваться с точкой шаблона.
		{"%s. Anime key visual.", "A cat on a roof.", "A cat on a roof. Anime key visual."},
		{"%s. Anime key visual.", "A cat on a roof", "A cat on a roof. Anime key visual."},
		{"%s. Anime key visual.", "  A cat on a roof ,", "A cat on a roof. Anime key visual."},
		{"%s", "A cat on a roof.", "A cat on a roof"},
		// Знаки внутри фразы не трогаем.
		{"%s. Neon.", "A cat, a dog and a roof.", "A cat, a dog and a roof. Neon."},
	}

	for _, c := range cases {
		if got := applyPromptTemplate(c[0], c[1]); got != c[2] {
			t.Errorf("applyPromptTemplate(%q, %q): want %q, got %q", c[0], c[1], c[2], got)
		}
	}
}
