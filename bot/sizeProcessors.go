package bot

import (
	"fmt"
	"log"
	rand "math/rand/v2"
	"strconv"
	"strings"
	"telebot/model"
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

// buildSizeText generates and returns the phrase to post for the user's daily
// size. The size is generated once per day (normal distribution around
// sizeMean, clamped to [sizeMin, sizeMax]) and stored, so repeated requests
// within the day yield the same number. The phrase template is taken from the
// DB by sizeKey and must contain the {size} placeholder; otherwise the number
// is appended. A random emotion emoji is appended at the end.
func buildSizeText(userID int64) string {
	generated := generateSize()

	size, err := model.GetOrCreateTodaySize(userID, generated)
	if err != nil {
		log.Println("[error] error getting user size", err)
		size = generated
	}

	template := "Карандаш у меня {size} см"
	phrazes, err := model.GetPharzesByKey(sizeKey, true)
	if err != nil {
		log.Println("[error] error getting size phrazes", err)
	}
	if phrazes != nil && len(*phrazes) > 0 {
		template = (*phrazes)[rand.IntN(len(*phrazes))].Value
	}

	return fmt.Sprintf("%s %s", formatSizePhrase(template, size), randomEmotionEmoji())
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
