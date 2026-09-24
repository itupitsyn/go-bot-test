package aiApi

import (
	"os"
	"time"
)

const (
	// The frame is vertical: clips are watched on phones, where a horizontal
	// one took up a strip in the middle of the screen. The short side of 768 is
	// the model's native resolution: below it loses detail, above it just takes
	// longer.
	//
	// The sides are multiples of 32: the service rounds the size down to a
	// multiple and would silently return something other than what was asked.
	// An exact 9:16 with a short side of 768 is not divisible by 32, so the
	// long side is 1344, a bit shorter than nine to sixteen.
	defaultVideoWidth  = 768
	defaultVideoHeight = 1344
	defaultVideoFps    = 24

	videoPollInterval = 10 * time.Second
	videoMaxWait      = time.Hour
)

// waitVideoResult polls the video service /api/result endpoint until the
// generation is finished and returns the decoded video bytes.
func waitVideoResult(id string, onProgress ProgressFunc) ([]byte, error) {
	return waitResult("video", os.Getenv("AI_VIDEO_HOST"), id, videoPollInterval, videoMaxWait, onProgress)
}
