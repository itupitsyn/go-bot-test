package aiApi

import (
	"strings"
	"testing"
)

func TestGetVideoTemplate(t *testing.T) {
	cases := [][2]string{
		{"кота медленно", "slow motion"},
		{"a cat slowly", "slow motion"},
		{"кота вокруг", "orbits the subject"},
		{"a cat around", "orbits the subject"},
		{"кота ближе", "push-in"},
		{"кота дальше", "pulls back"},
		{"кота сверху", "aerial shot"},
		{"кота живо", "energetic action"},
		{"Кота МЕДЛЕННО", "slow motion"},
	}

	for _, c := range cases {
		res := getVideoTemplate(c[0])
		if !strings.Contains(res, c[1]) {
			t.Errorf("getVideoTemplate(%q): want it to include %q, got %q", c[0], c[1], res)
		}
	}
}

func TestGetVideoTemplateFallsBackToDefault(t *testing.T) {
	cases := []string{"кота", "", "танцующего кота на столе"}

	for _, c := range cases {
		if res := getVideoTemplate(c); res != defaultVideoTemplate {
			t.Errorf("getVideoTemplate(%q): want the default template, got %q", c, res)
		}
	}
}

func TestEveryVideoTemplateMentionsSound(t *testing.T) {
	// H3 generates stereo in the same pass, so the track ends up in the mp4
	// regardless of us: every template must talk about it.
	if !strings.Contains(defaultVideoTemplate, "sound") {
		t.Errorf("the default template says nothing about sound: %q", defaultVideoTemplate)
	}

	for _, style := range videoStyles {
		lower := strings.ToLower(style.template)
		if !strings.Contains(lower, "sound") && !strings.Contains(lower, "ambience") {
			t.Errorf("template %q says nothing about sound", style.template)
		}
	}
}

func TestEveryVideoTemplateHasOnePlaceholder(t *testing.T) {
	templates := append([]string{defaultVideoTemplate}, func() []string {
		out := []string{}
		for _, style := range videoStyles {
			out = append(out, style.template)
		}
		return out
	}()...)

	for _, template := range templates {
		if strings.Count(template, "%s") != 1 {
			t.Errorf("template %q must carry exactly one %%s", template)
		}
	}
}

func TestGetVideoPrompt(t *testing.T) {
	cases := [][2]string{
		{"кота медленно", "кота"},
		{"танцующего кота вокруг", "танцующего кота"},
		{"a dancing cat slowly", "a dancing cat"},
		{"Кота СВЕРХУ", "кота"},
		{"кота", "кота"},
		{"", ""},
		// The keyword is separated by a space, so a word that merely ends with
		// it stays whole.
		{"кота наживо", "кота наживо"},
		{"кота вокруг вокруг", "кота вокруг"},
	}

	for _, c := range cases {
		if res := getVideoPrompt(c[0]); res != c[1] {
			t.Errorf("getVideoPrompt(%q): want %q, got %q", c[0], c[1], res)
		}
	}
}

func TestGetVideoPromptLeavesNothingWhenOnlyKeyword(t *testing.T) {
	// Only emptiness is left: the caller must substitute the default subject,
	// otherwise a bare template goes to the model.
	if res := getVideoPrompt("медленно"); res != "медленно" {
		t.Errorf("одинокое слово без пробела перед ним ключевым не считается, got %q", res)
	}
}
