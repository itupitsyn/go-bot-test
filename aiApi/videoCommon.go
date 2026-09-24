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
	// silently return something other than what was asked. 576x1024 satisfies
	// that and is an exact 9:16, so nothing is cropped on a phone.
	//
	// Was 768x1344 until 24.09.2026. Generation time grows roughly as pixels
	// to the power of 1.3, and that canvas cost six minutes a clip:
	//
	//     768x1344  1032k px  351 s
	//     576x1024   589k px  161 s
	//     512x896    458k px  105 s
	//
	// (measured back to back on one card, 8 steps, MiniMax H3). Every t2v the
	// bot had ever sent ran 378-415 s; the faster numbers in the service log
	// are bench runs at 832x480, not real traffic. 512x896 is the next step
	// down if 161 s is still too slow; its picture quality was not compared.
	defaultVideoWidth  = 576
	defaultVideoHeight = 1024
	defaultVideoFps    = 24

	videoPollInterval = 10 * time.Second
	videoMaxWait      = time.Hour
)

// waitVideoResult polls the video service /api/result endpoint until the
// generation is finished and returns the decoded video bytes.
func waitVideoResult(id string, onProgress ProgressFunc) ([]byte, error) {
	return waitResult("video", os.Getenv("AI_VIDEO_HOST"), id, videoPollInterval, videoMaxWait, onProgress)
}
