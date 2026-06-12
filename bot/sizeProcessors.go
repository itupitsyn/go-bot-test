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

// buildSizeText generates and returns the phrase to post for the user's daily
// size. The size is generated once per day (1..50) and stored, so repeated
// requests within the day yield the same number. The phrase template is taken
// from the DB by sizeKey and must contain the {size} placeholder; otherwise the
// number is appended.
func buildSizeText(userID int64) string {
	generated := rand.IntN(50) + 1 // 1..50

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

	return formatSizePhrase(template, size)
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
