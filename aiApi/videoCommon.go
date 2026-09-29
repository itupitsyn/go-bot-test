package aiApi

import (
	"os"
	"time"
)

const (
	// The frame is vertical: clips are watched on phones, where a horizontal
	// one took up a strip in the middle of the screen.
	//
	// The sides must be multiples of 32 -- the service rounds down and would
	// silently return something other than what was asked.
	//
	// Was 768x1344 until 24.09.2026, then 576x1024 until 30.09.2026. Generation
	// time grows roughly as pixels to the power of 1.39, and the big canvas
	// cost six minutes a clip:
	//
	//     768x1344  1032k px  351 s   measured
	//     672x896    602k px  ~166 s  PREDICTED from the measured points
	//     576x1024   589k px  161 s   measured
	//     512x896    458k px  105 s   measured
	//
	// (measured back to back on one card, 8 steps, MiniMax H3). Every t2v the
	// bot had ever sent ran 378-415 s; the faster numbers in the service log
	// are bench runs at 832x480, not real traffic.
	//
	// 672x896 is 3:4 instead of the old 9:16: 576 was too narrow to fit a scene
	// into. The pixel count is deliberately kept at the old level (+2%), so the
	// clip is 17% wider but 12% shorter and costs the same time -- widening at
	// a constant budget cannot do otherwise. The service caps at 1344*768 px
	// and snaps to a multiple of 32; both sides here already are, so
	// h3_snap_size passes them through untouched.
	defaultVideoWidth  = 672
	defaultVideoHeight = 896
	defaultVideoFps    = 24

	videoPollInterval = 10 * time.Second
	videoMaxWait      = time.Hour
)

// waitVideoResult polls the video service /api/result endpoint until the
// generation is finished and returns the decoded video bytes.
func waitVideoResult(id string, onProgress ProgressFunc) ([]byte, error) {
	return waitResult("video", os.Getenv("AI_VIDEO_HOST"), id, videoPollInterval, videoMaxWait, onProgress)
}
