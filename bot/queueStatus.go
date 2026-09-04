package bot

import (
	"fmt"
	"telebot/aiApi"
	"time"
)

// formatQueueStatus говорит, чего ждать. Пока задача стоит в очереди — сколько
// впереди народу; как только за неё взялись (или впереди никого и не было),
// очередь кончилась и остаётся обычное «жди».
//
// Пустую очередь важно не спутать с «один впереди»: свободная карта успевает
// взять задачу не мгновенно, и в этот зазор бот не должен объявлять очередь
// из одного себя.
func formatQueueStatus(status aiApi.QueueStatus, waitPhrase string) string {
	if status.Running || status.Ahead == 0 {
		return waitPhrase
	}

	return fmt.Sprintf("Ты %d-й в очереди, %s", status.Ahead+1, formatEta(status.ETA))
}

// formatEta переводит оценку ожидания в человеческие слова. Минуты округляем:
// точность тут мнимая, ждущему важен порядок величины.
func formatEta(eta time.Duration) string {
	minutes := int(eta.Round(time.Minute).Minutes())
	if minutes < 1 {
		return "уже скоро"
	}

	return fmt.Sprintf("минут %d", minutes)
}
