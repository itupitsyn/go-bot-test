package bot

import (
	"math"
	rand "math/rand/v2"
)

// emotionEmojis are appended to the end of a phrase to convey a random emotion
// — pleasure, distress, smugness, etc.
var emotionEmojis = []string{
	"😍", "😋", "🤤", "🥵", "😏", "😈", "🥴", "🫠",
	"😩", "😭", "😖", "🥺", "😤", "😳", "😵‍💫", "🤯",
}

// randomEmotionEmoji returns a random emotion emoji from emotionEmojis.
func randomEmotionEmoji() string {
	return emotionEmojis[rand.IntN(len(emotionEmojis))]
}

// clampedNormal draws an integer from a normal distribution centred on mean
// (rounded to the nearest integer) and clamps it to [min, max], so mean is the
// most probable outcome.
func clampedNormal(mean, stdDev float64, min, max int) int {
	value := int(math.Round(rand.NormFloat64()*stdDev + mean))
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
