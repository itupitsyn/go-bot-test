package aiApi

import (
	"os"
	"time"
)

const (
	// Кадр вертикальный: ролики смотрят с телефона, и горизонтальный занимал
	// там полоску посреди экрана. Короткая сторона 768 — родное разрешение
	// модели, ниже она теряет в детализации, выше просто дольше считает.
	//
	// Стороны кратны 32: сервис режет размер до кратного и молча выдал бы не
	// то, что просили. Ровные 9:16 при короткой стороне 768 на 32 не делятся,
	// поэтому длинная — 1344, чуть короче девяти к шестнадцати.
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
