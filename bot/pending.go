package bot

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"sync"
	"telebot/aiApi"
	"telebot/model"
	"telebot/utils"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Журнал незабранного: второй слой защиты от потерь.
//
// ПЕРВЫЙ слой — ожидание обработчиков при остановке — покрывает только
// плановый перезапуск, и то не весь: бот ждёт ограниченное время, а задача
// вместе с очередью живёт дольше. Замеры за сутки 14.10.2026: средний i2v 275
// секунд, средний t2v 354, максимальное ожидание в очереди 1025. Падение, OOM
// и снятие питания не пережидает вообще ничто.
//
// ЗДЕСЬ опора на другое: генерация живёт НЕ В БОТЕ. Сервис досчитывает задачу,
// даже если бот умер, и держит результат шесть часов, не удаляя при чтении.
// Значит, любая смерть бота теряет не работу, а только доставку — а доставку
// можно повторить, если задача записана. Запись появляется в ту секунду, когда
// сервис назвал задачу, и исчезает, когда обработчик дошёл до конца любым
// путём.
//
// Профилактику подбор НЕ СМОТРИТ, и это нарочно. Профилактика отказывает в
// новой работе, а здесь работа давно сделана и оплачена; молчать о готовом
// результате потому, что в этот момент идёт деплой, было бы издевательством.

const (
	// resumeTTL — до каких пор задачу есть смысл подбирать. Сервис держит
	// готовое шесть часов; берём пять с запасом, чтобы не гнаться за тем, что
	// исчезнет у нас из-под рук, и не тянуть с возвратом денег.
	resumeTTL = 5 * time.Hour
	// resumeLimit — сколько ждать подобранную задачу. Полчаса покрывают самое
	// долгое, что видели: 1025 секунд в очереди плюс счёт.
	resumeLimit = 30 * time.Minute
)

const (
	// recoveredCaption объясняет задержку. Без него человек получает видео
	// неизвестно на что и через полчаса после того, как перестал ждать.
	recoveredCaption = "Это твоя генерация — она досчиталась, пока бота перезапускали."
	// resumeLostText — когда результата уже нет. Про возврат говорим только
	// если он действительно был: обещать возврат там, где не списывали,
	// значит путать человека.
	resumeLostText     = "Твоя генерация не доехала: бота перезапустили, а результат на сервисе уже не хранится."
	resumeRefundedText = resumeLostText + " Списанное вернул."
)

// pendingNote — расписка: бот должен человеку результат.
//
// Живёт в обработчике и ставится через defer, поэтому снимается на ЛЮБОМ
// выходе — и удачном, и по ошибке. Запись остаётся только тогда, когда
// обработчик не дошёл до конца вовсе, то есть когда процесс умер. Это и есть
// тот случай, ради которого журнал существует.
type pendingNote struct {
	row model.PendingDelivery

	mu sync.Mutex
	id string
}

// newPendingNote collects everything needed to hand the result over later.
func newPendingNote(kind string, message *models.Message, wait *waitMessage, charge *aiCharge) *pendingNote {
	note := &pendingNote{row: model.PendingDelivery{
		Kind:      kind,
		ChatID:    message.Chat.ID,
		ReplyToID: message.ID,
		WaitID:    wait.id(),
		PaidUnits: charge.paidUnits(),
	}}
	if message.From != nil {
		note.row.UserID = message.From.ID
	}

	return note
}

// onJobID goes into aiApi.Caller.OnJobID: the service has named the job.
func (n *pendingNote) onJobID(id string) {
	n.mu.Lock()
	n.id = id
	row := n.row
	n.mu.Unlock()

	row.JobID = id
	if err := model.RememberDelivery(row); err != nil {
		// Не повод прерывать генерацию: без записи мы теряем страховку, а с
		// прерыванием — саму работу, за которую уже заплачено.
		log.Println("[error] не записал задачу в журнал незабранного")
		log.Println(err)
	}
}

// done closes the note: the handler got to the end, nobody has to pick this up.
func (n *pendingNote) done() {
	n.mu.Lock()
	id := n.id
	n.mu.Unlock()

	if id == "" {
		return
	}

	forgetDelivery(id)
}

func forgetDelivery(id string) {
	if err := model.ForgetDelivery(id); err != nil {
		log.Println("[error] не убрал задачу из журнала незабранного")
		log.Println(err)
	}
}

// RecoverDeliveries hands over the work the previous process did not manage to.
//
// Вызывается один раз при старте. Каждая задача идёт своей горутиной: они
// независимы, а ждать их по очереди значило бы, что вторая получит свой
// результат через полчаса после первой.
func RecoverDeliveries(ctx context.Context, b *bot.Bot) {
	owed, err := model.OwedDeliveries()
	if err != nil {
		log.Println("[error] не прочитал журнал незабранного")
		log.Println(err)

		return
	}

	if len(owed) == 0 {
		return
	}

	log.Printf("Незабранных задач в журнале: %d, подбираю\n", len(owed))
	for _, row := range owed {
		go resumeDelivery(ctx, b, row)
	}
}

// resumeDelivery sees one abandoned job through: waits for it and hands it over,
// or gives up and gives the money back.
func resumeDelivery(ctx context.Context, b *bot.Bot, row model.PendingDelivery) {
	// Считаемся наравне с обработчиками: следующая остановка должна знать, что
	// кто-то ещё не отдал результат, и подождать.
	inFlight.Add(1)
	defer inFlight.Add(-1)

	// Свой контекст, по той же причине, что у обработчиков: родительский
	// отменят по сигналу, а отдавать готовое всё равно надо.
	ctx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx), resumeLimit+time.Minute)
	defer cancel()

	if age := time.Since(row.CreatedAt); age > resumeTTL {
		log.Printf("Задача %s брошена %s назад — результата на сервисе уже нет\n",
			row.JobID, age.Round(time.Minute))
		giveUpDelivery(ctx, b, row)

		return
	}

	data, err := aiApi.Resume(row.Kind, row.JobID, resumeLimit)
	if err != nil {
		log.Printf("[error] не дождался подобранной задачи %s: %v\n", row.JobID, err)
		giveUpDelivery(ctx, b, row)

		return
	}

	if err := handOverDelivery(ctx, b, row, data); err != nil {
		log.Printf("[error] подобранная задача %s не доехала: %v\n", row.JobID, err)
		giveUpDelivery(ctx, b, row)

		return
	}

	log.Printf("Отдал подобранную задачу %s (%s), %d байт\n", row.JobID, row.Kind, len(data))
	forgetDelivery(row.JobID)
}

// resumedMedia wraps the result the way its kind requires. The reader is made
// fresh on every call on purpose: deliver repeats the attempt, and a reader
// already read to the end would send nothing.
func resumedMedia(kind string, data []byte) (models.InputMedia, error) {
	switch kind {
	case "image", "edit":
		return &models.InputMediaPhoto{
			Media:           "attach://image.png",
			MediaAttachment: bytes.NewReader(data),
			HasSpoiler:      true,
			Caption:         recoveredCaption,
		}, nil
	case "video":
		return &models.InputMediaVideo{
			Media:           "attach://image.mp4",
			MediaAttachment: bytes.NewReader(data),
			HasSpoiler:      true,
			Caption:         recoveredCaption,
		}, nil
	}

	return nil, fmt.Errorf("неизвестный вид задачи %q", kind)
}

// handOverDelivery sends the picked-up result to the chat it was asked from.
func handOverDelivery(ctx context.Context, b *bot.Bot, row model.PendingDelivery, data []byte) error {
	_, err := deliver(ctx, "подобранный результат", func() ([]*models.Message, error) {
		media, mediaErr := resumedMedia(row.Kind, data)
		if mediaErr != nil {
			return nil, mediaErr
		}

		return b.SendMediaGroup(ctx, &bot.SendMediaGroupParams{
			ChatID:          row.ChatID,
			Media:           []models.InputMedia{media},
			ReplyParameters: resumeReply(row.ReplyToID),
		})
	})
	if err != nil {
		return err
	}

	dropWaitMessage(ctx, b, row)

	return nil
}

// resumeReply replies to the original question when it is still there.
//
// AllowSendingWithoutReply обязателен: между вопросом и подобранным ответом
// прошли часы, и сообщение могли удалить. Без этого флага Telegram отказал бы
// в отправке целиком, и мы потеряли бы результат из-за пропавшего вопроса.
func resumeReply(messageID int) *models.ReplyParameters {
	if messageID == 0 {
		return nil
	}

	return &models.ReplyParameters{
		MessageID:                messageID,
		AllowSendingWithoutReply: true,
	}
}

// dropWaitMessage removes the stale "жди теперь": by now it has been hanging
// there since the previous process, and it is a lie.
func dropWaitMessage(ctx context.Context, b *bot.Bot, row model.PendingDelivery) {
	if row.WaitID == 0 {
		return
	}

	if _, err := b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    row.ChatID,
		MessageID: row.WaitID,
	}); err != nil {
		// Ошибка здесь ничего не стоит: сообщение могли уже удалить руками.
		log.Printf("[warn] не убрал ждущее сообщение %d: %v\n", row.WaitID, err)
	}
}

// giveUpDelivery admits the result is gone, gives the money back and says so.
//
// Возврат ИДЁТ ПЕРВЫМ, запись снимается после. Умри процесс между ними — мы
// вернём деньги дважды при следующем старте, и это осознанный выбор: лишняя
// генерация стоит нам дешевле, чем человеку — отобранные звёзды.
func giveUpDelivery(ctx context.Context, b *bot.Bot, row model.PendingDelivery) {
	text := resumeLostText

	if row.PaidUnits > 0 {
		if err := model.AddAiCredit(row.UserID, row.PaidUnits); err != nil {
			// Та же строка, что и в setUnits: это единственный след, по
			// которому потом разобраться.
			log.Printf("[error] не вернул %d оплаченных генераций пользователю %d: %v\n",
				row.PaidUnits, row.UserID, err)
		} else {
			text = resumeRefundedText
			log.Printf("Вернул %d оплаченных генераций пользователю %d за потерянную %s\n",
				row.PaidUnits, row.UserID, row.JobID)
		}
	}

	dropWaitMessage(ctx, b, row)

	_, err := deliver(ctx, "известие о потере", func() (*models.Message, error) {
		return b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          row.ChatID,
			Text:            text,
			ReplyParameters: resumeReply(row.ReplyToID),
		})
	})
	utils.ProcessSendMessageError(err, row.ChatID)

	forgetDelivery(row.JobID)
}
