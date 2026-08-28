package aiApi

import (
	"os"
	"time"
)

const (
	defaultVideoWidth  = 832
	defaultVideoHeight = 480
	defaultVideoFps    = 24

	videoPollInterval = 10 * time.Second
	videoMaxWait      = time.Hour
)

// waitVideoResult polls the video service /api/result endpoint until the
// generation is finished and returns the decoded video bytes.
func waitVideoResult(id string) ([]byte, error) {
	return waitResult("video", os.Getenv("AI_VIDEO_HOST"), id, videoPollInterval, videoMaxWait)
}
