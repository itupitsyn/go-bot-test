package bot

import (
	"context"
	"fmt"
	"log"
	"telebot/model"
	"telebot/utils"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Сколько осталось.
//
// До баллов спрашивать было нечего: генерация стоила одну генерацию, и
// бесплатное от купленного человек отличал по тому, отказали ему или нет.
// Теперь у чисел разная цена, и не видеть их — значит считать в уме.

// aiPeriodWords — как назвать окно лимита в ответе. Неизвестный период
// подписываем нейтрально: лучше без срока, чем с выдуманным.
var aiPeriodWords = map[string]string{
	model.AiPeriodHour: "в час",
	model.AiPeriodDay:  "в сутки",
	model.AiPeriodWeek: "в неделю",
}

// aiBalanceText says what is left of the free quota and of the bought points.
//
// Бесплатное и купленное показываем отдельно, а не суммой: тратятся они в
// разном порядке (сначала квота, потом карман), и сумма соврала бы о том,
// сколько человек успеет до того, как начнёт платить.
func aiBalanceText(userID int64) string {
	limit, err := model.GetAiLimit(userID)
	if err != nil {
		log.Println("[error] баланс: лимит не прочитался:", err)

		return serverDeadText
	}
	if !limit.IsActive() {
		return aiBuyPointlessText
	}

	free := limit.Amount
	used, err := model.CountAiUsage(userID, time.Now().Add(-model.AiLimitWindow(limit.Period)))
	if err != nil {
		log.Println("[error] баланс: расход не прочитался:", err)

		return serverDeadText
	}
	free -= used
	if free < 0 {
		free = 0
	}

	balance := 0
	credit, err := model.GetAiCredit(userID)
	if err != nil {
		log.Println("[error] баланс: карман не прочитался:", err)

		return serverDeadText
	}
	if credit != nil {
		balance = credit.Balance
	}

	text := fmt.Sprintf("Бесплатных баллов осталось %d из %d %s.", free, limit.Amount,
		aiPeriodWords[limit.Period])
	if balance > 0 {
		text += fmt.Sprintf(" Купленных на счету: %d — они не сгорают и тратятся после бесплатных.", balance)
	}

	return text + fmt.Sprintf(
		"\n\nПравка картинки — %d, картинка с нуля — %d, ролик — %d (длинный, на %.0f секунд, — %d), стикерпак — %d.",
		pointsEdit, pointsImage, aiPoints(aiKindVideo),
		videoSecondsMax, aiVideoPoints(videoSecondsMax), 10*pointsSticker)
}

// processAiBalance answers how much is left.
//
// В группе чисел не называем, как и при отказе: квота у каждого своя, и
// вслух это спор о том, почему у соседа больше (см. aiLimitFallbackText).
func processAiBalance(ctx context.Context, b *bot.Bot, message *models.Message) {
	chatID := message.Chat.ID
	text := aiBalanceText(message.From.ID)
	if message.Chat.Type != "private" {
		text = fmt.Sprintf("Скажу в личке, чтобы не считать вслух чужие баллы: @%s", botName)
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatID,
		Text:            text,
		ReplyParameters: &models.ReplyParameters{MessageID: message.ID},
	})
	utils.ProcessSendMessageError(err, chatID)
}
