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
