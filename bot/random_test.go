package bot

import "testing"

func TestRandomEmotionEmojiFromSet(t *testing.T) {
	set := make(map[string]bool, len(emotionEmojis))
	for _, e := range emotionEmojis {
		set[e] = true
	}
	for i := 0; i < 1000; i++ {
		if got := randomEmotionEmoji(); !set[got] {
			t.Fatalf("randomEmotionEmoji() = %q, not in emotionEmojis", got)
		}
	}
}
