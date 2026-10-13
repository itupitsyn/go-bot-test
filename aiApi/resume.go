package aiApi

import (
	"fmt"
	"os"
	"time"
)

// Подбор брошенной работы.
//
// Сервис не узнаёт о смерти бота: он досчитывает задачу до конца и держит
// результат шесть часов (RESULT_TTL_HOURS на его стороне), а /api/result не
// удаляет запись при чтении — это сделано там нарочно, ради обрыва связи.
// Значит, смерть бота посреди ожидания теряет не работу, а только доставку, и
// дождаться задачи можно заново, по имени.
//
// Проверено вживую 13.10.2026: видео 4d1ae46d досчиталось на сервисе через
// шесть минут после того, как перезапуск убил ожидавший его обработчик, и
// лежало готовым, пока не истёк срок.

// Resume waits for a job someone else started — usually a previous process —
// and returns its result. `kind` is what the generation calls itself:
// "image", "edit" or "video".
//
// limit ограничивает ожидание сверху. У подобранной задачи нет человека,
// который смотрит на часы, но висеть вечно она не должна: пока ждём, строка в
// журнале незабранного занята, а значит, задача не будет подобрана снова.
func Resume(kind, id string, limit time.Duration) ([]byte, error) {
	switch kind {
	case "image", "edit":
		return waitResult(kind, os.Getenv("AI_PAINTER_HOST"), id, imagePollInterval, limit, nil)
	case "video":
		return waitResult(kind, os.Getenv("AI_VIDEO_HOST"), id, videoPollInterval, limit, nil)
	}

	// Не паника и не молчание: неизвестный вид означает, что в базе лежит
	// строка, которую некому отдать, и про неё надо узнать.
	return nil, fmt.Errorf("unknown job kind %q", kind)
}
