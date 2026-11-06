package bot

import (
	"fmt"
	"log"
	rand "math/rand/v2"
	"strconv"
	"strings"
	"telebot/model"
	"unicode"
)

const sizeKey = "size"
const sizePlaceholder = "{size}"

// Size values are drawn from a normal distribution centred on sizeMean (the
// most probable value), then clamped to [sizeMin, sizeMax].
const (
	sizeMean   = 20.0
	sizeStdDev = 10.0
	sizeMin    = 1
	sizeMax    = 40
)

// maxSizeObjectRunes keeps a whole typed sentence from becoming the "object".
const maxSizeObjectRunes = 60

// buildSizeText generates and returns the phrase to post for the user's daily
// size. The size is generated once per day (normal distribution around
// sizeMean, clamped to [sizeMin, sizeMax]) and stored, so repeated requests
// within the day yield the same number. The phrase template is taken from the
// DB by sizeKey and must contain the {size} placeholder; otherwise the number
// is appended. A random emotion emoji is appended at the end.
//
// object is what the person typed into the inline query. When there is one,
// it replaces the object of the DB phrases: "огурец" gives "Огурец у меня 17
// см". The number stays the day's number either way.
func buildSizeText(userID int64, object string) string {
	generated := generateSize()

	size, err := model.GetOrCreateTodaySize(userID, generated)
	if err != nil {
		log.Println("[error] error getting user size", err)
		size = generated
	}

	template, ok := customSizeTemplate(object)
	if !ok {
		template = "Карандаш у меня {size} см"
		phrazes, err := model.GetPharzesByKey(sizeKey, true)
		if err != nil {
			log.Println("[error] error getting size phrazes", err)
		}
		if phrazes != nil && len(*phrazes) > 0 {
			template = (*phrazes)[rand.IntN(len(*phrazes))].Value
		}
	}

	return fmt.Sprintf("%s %s", formatSizePhrase(template, size), randomEmotionEmoji())
}

// sizeObject tidies what was typed: one line, single spaces, not too long.
func sizeObject(object string) string {
	object = strings.Join(strings.Fields(object), " ")
	if runes := []rune(object); len(runes) > maxSizeObjectRunes {
		object = strings.TrimSpace(string(runes[:maxSizeObjectRunes]))
	}

	return object
}

// customSizeTemplate builds the phrase around the person's own object, in the
// same shape as every DB phrase ("Морковка у меня {size} см"). The first letter
// is raised because the object starts the sentence. False when nothing was
// typed.
func customSizeTemplate(object string) (string, bool) {
	object = sizeObject(object)
	if object == "" {
		return "", false
	}

	runes := []rune(object)
	runes[0] = unicode.ToUpper(runes[0])

	return string(runes) + " у меня " + sizePlaceholder + " см", true
}

// generateSize draws a daily size from a normal distribution centred on
// sizeMean, clamped to [sizeMin, sizeMax].
func generateSize() int {
	return clampedNormal(sizeMean, sizeStdDev, sizeMin, sizeMax)
}

// formatSizePhrase substitutes the size into the template. If the template
// contains the {size} placeholder it is replaced; otherwise the number is
// appended with the "см" unit.
func formatSizePhrase(template string, size int) string {
	if strings.Contains(template, sizePlaceholder) {
		return strings.ReplaceAll(template, sizePlaceholder, strconv.Itoa(size))
	}
	return fmt.Sprintf("%s %d см", template, size)
}
