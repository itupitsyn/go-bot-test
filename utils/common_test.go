package utils

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestGetAlternativeNameFull(t *testing.T) {
	usr := models.User{
		FirstName: "John",
		LastName:  "Malkovich",
	}

	res := GetAlternativeName(&usr)
	if res != "JohnMalkovich" {
		t.Errorf("want JohnMalkovich, got %s", res)
	}
}

func TestGetAlternativeNameFirstNameOnly(t *testing.T) {
	usr := models.User{
		FirstName: "John",
	}

	res := GetAlternativeName(&usr)
	if res != "John" {
		t.Errorf("want John, got %s", res)
	}
}

func TestGetAlternativeNameLastNameOnly(t *testing.T) {
	usr := models.User{
		LastName:  "Malkovich",
	}

	res := GetAlternativeName(&usr)
	if res != "Malkovich" {
		t.Errorf("want Malkovich, got %s", res)
	}
}

func TestGetAlternativeNameEmpty(t *testing.T) {
	usr := models.User{
	}

	res := GetAlternativeName(&usr)
	if res != "" {
		t.Errorf("want \"\", got \"%s\"", res)
	}
}

func TestGetAnyNameUsername(t *testing.T) {
	usr := models.User{
		Username:  "johnny",
		FirstName: "John",
		LastName:  "Malkovich",
	}

	res := GetAnyName(&usr)
	if res != "johnny" {
		t.Errorf("want johnny, got %s", res)
	}
}

func TestGetAnyNameFallback(t *testing.T) {
	usr := models.User{
		FirstName: "John",
		LastName:  "Malkovich",
	}

	res := GetAnyName(&usr)
	if res != "JohnMalkovich" {
		t.Errorf("want JohnMalkovich, got %s", res)
	}
}

func TestTrimPrefixIgnoreCase(t *testing.T) {
	cases := [][3]string{
		{"DRAW cat", "draw ", "cat"},
		{"Нарисуй кота", "нарисуй ", "кота"},
		{"hello world", "draw ", "hello world"},
		{"draw", "draw ", "draw"},
	}

	for _, c := range cases {
		res := TrimPrefixIgnoreCase(c[0], c[1])
		if res != c[2] {
			t.Errorf("TrimPrefixIgnoreCase(%q, %q): want %q, got %q", c[0], c[1], c[2], res)
		}
	}
}
