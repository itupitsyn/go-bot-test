package bot

import (
	"fmt"
	"log"
	rand "math/rand/v2"
	"strconv"
	"strings"
	"telebot/model"
)

const depthKey = "depth"
const depthPlaceholder = "{depth}"

// Depth values are drawn from a normal distribution centred on depthMean (the
// most probable value), then clamped to [depthMin, depthMax].
const (
	depthMean   = 20.0
	depthStdDev = 10.0
	depthMin    = 1
	depthMax    = 40
)

// buildDepthText generates and returns the phrase to post for the user's daily
// Matilda depth. The depth is generated once per day (normal distribution
// around depthMean, clamped to [depthMin, depthMax]) and stored, so repeated
// requests within the day yield the same number. The phrase template is taken
// from the DB by depthKey and must contain the {depth} placeholder; otherwise
// the number is appended. A random emotion emoji is appended at the end.
func buildDepthText(userID int64) string {
	generated := generateDepth()

	depth, err := model.GetOrCreateTodayDepth(userID, generated)
	if err != nil {
		log.Println("[error] error getting user depth", err)
		depth = generated
	}

	template := "Матильда у меня {depth} см"
	phrazes, err := model.GetPharzesByKey(depthKey, true)
	if err != nil {
		log.Println("[error] error getting depth phrazes", err)
	}
	if phrazes != nil && len(*phrazes) > 0 {
		template = (*phrazes)[rand.IntN(len(*phrazes))].Value
	}

	return fmt.Sprintf("%s %s", formatDepthPhrase(template, depth), randomEmotionEmoji())
}

// generateDepth draws a daily depth from a normal distribution centred on
// depthMean, clamped to [depthMin, depthMax].
func generateDepth() int {
	return clampedNormal(depthMean, depthStdDev, depthMin, depthMax)
}

// formatDepthPhrase substitutes the depth into the template. If the template
// contains the {depth} placeholder it is replaced; otherwise the number is
// appended with the "м" unit.
func formatDepthPhrase(template string, depth int) string {
	if strings.Contains(template, depthPlaceholder) {
		return strings.ReplaceAll(template, depthPlaceholder, strconv.Itoa(depth))
	}
	return fmt.Sprintf("%s %d см", template, depth)
}
