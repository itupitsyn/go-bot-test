package bot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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
// можно повторить, если задача записана.
//
// Проверено живьём 14.10.2026 в 02:43: kill -9 посреди t2v, через три секунды
// новый процесс, «Незабранных задач в журнале: 1, подбираю», и в 02:45:47 —
// «Отдал подобранную задачу faba1ce1… (video), 1412611 байт». Процесс, который
// отдал видео, никогда его не заказывал.
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

// unnamedJobPrefix отмечает задачу, которую сервис ещё не назвал.
//
// ЗАЧЕМ ЭТО ВООБЩЕ ЕСТЬ. Списание идёт ДО работы (см. chargeAiLimit), а имя
// задачи появляется только после отправки на сервис. Между этими двумя
// событиями успевает пройти скачивание фото из Telegram и перевод промта через
// LLM — по журналу это 1.4 секунды на t2v и около трёх на i2v, а при занятой
// LLM и больше. Умри процесс в этом промежутке — деньги списаны, а записи нет
// вовсе, и возвращать их потом некому и нечем.
//
// Поэтому расписка пишется СРАЗУ, с нашим собственным именем, а настоящее
// подставляется, когда сервис его назовёт. Строка с таким именем означает
// «задача потеряна до того, как обрела имя»: подобрать её нельзя — спрашивать
// сервис не о чем, — но деньги вернуть можно, и это главное.
const unnamedJobPrefix = "unnamed:"

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
// Живёт в обработчике и снимается через defer, то есть на ЛЮБОМ выходе — и
// удачном, и по ошибке. Запись остаётся только тогда, когда обработчик не
// дошёл до конца вовсе, то есть когда процесс умер. Это и есть тот случай,
// ради которого журнал существует.
type pendingNote struct {
	row model.PendingDelivery

	mu sync.Mutex
	id string
}

// newPendingNote collects everything needed to hand the result over later. It
// touches no database: writing is open's job, so that the parts that decide
// what to remember can be tested on their own.
func newPendingNote(kind string, message *models.Message, wait *waitMessage, charge *aiCharge) *pendingNote {
	var userID int64
	if message != nil && message.From != nil {
		userID = message.From.ID
	}

	note := &pendingNote{row: model.PendingDelivery{
		JobID:     unnamedJob(userID),
		Kind:      kind,
		WaitID:    wait.id(),
		UserID:    userID,
		PaidUnits: charge.paidUnits(),
	}}
	if message != nil {
		note.row.ChatID = message.Chat.ID
		note.row.ReplyToID = message.ID
	}
	note.id = note.row.JobID

	return note
}

// newInlinePendingNote is the same for a generation started by pressing the
// button under an inline result.
//
// Денег тут нет и возвращать нечего: инлайн-путь не проходит через списание
// вовсе — ни квоты, ни звёзд. Зато результат такой же настоящий, и терять его
// так же обидно, поэтому расписка нужна.
//
// Вместо чата и сообщения-вопроса — инлайн-сообщение: оно и есть то место,
// куда кладётся результат, и его идентификатор остаётся годным и через часы.
func newInlinePendingNote(kind, inlineMessageID string, userID int64, caption string) *pendingNote {
	note := &pendingNote{row: model.PendingDelivery{
		JobID:           unnamedJob(userID),
		Kind:            kind,
		InlineMessageID: inlineMessageID,
		Caption:         caption,
		UserID:          userID,
	}}
	note.id = note.row.JobID

	return note
}

// unnamedSeq делает временные имена различимыми независимо от часов.
//
// Сначала я взял только UnixNano и понадеялся, что в одну наносекунду два
// запроса не попадут. Собственный тест это и опроверг сразу: часы тикают с
// шагом много крупнее наносекунды, и два вызова подряд вернули одно значение.
// Последствие было бы неприятным — имя это первичный ключ, и вторая расписка
// перетёрла бы первую, то есть одна из задач осталась бы без страховки.
var unnamedSeq atomic.Uint64

// unnamedJob makes up a name for a job the service has not named yet.
func unnamedJob(userID int64) string {
	return fmt.Sprintf("%s%d:%d:%d", unnamedJobPrefix, userID,
		time.Now().UnixNano(), unnamedSeq.Add(1))
}

// isUnnamed says whether the job died before the service gave it a name.
//
// Такую подбирать нельзя: имени, по которому спрашивать результат, не
// существует. Зато деньги за неё списаны, и вернуть их — единственное, что тут
// можно и нужно сделать.
func isUnnamed(jobID string) bool {
	return strings.HasPrefix(jobID, unnamedJobPrefix)
}

// open writes the note down. From this moment the job survives the death of the
// process — at first only as "somebody is owed something", and with a real name
// as soon as the service gives one.
func (n *pendingNote) open() {
	n.mu.Lock()
	row := n.row
	n.mu.Unlock()

	if err := model.RememberDelivery(row); err != nil {
		// Не повод прерывать генерацию: без записи мы теряем страховку, а с
		// прерыванием — саму работу, за которую уже заплачено.
		log.Println("[error] не записал задачу в журнал незабранного")
		log.Println(err)
	}
}

// onJobID goes into aiApi.Caller.OnJobID: the service has named the job, so the
// note gets its real name.
func (n *pendingNote) onJobID(id string) {
	n.mu.Lock()
	was := n.id
	n.id = id
	n.row.JobID = id
	row := n.row
	n.mu.Unlock()

	if was == id {
		return
	}

	switched, err := model.RenameDelivery(was, id)
	if err != nil {
		log.Println("[error] не переименовал задачу в журнале незабранного")
		log.Println(err)

		return
	}

	if !switched {
		// Строки не было: запись при старте не удалась. Пишем теперь — с
		// настоящим именем она даже полезнее.
		if err := model.RememberDelivery(row); err != nil {
			log.Println("[error] не записал названную задачу в журнал незабранного")
			log.Println(err)
		}
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
		utils.Go("resumeDelivery", func() { resumeDelivery(ctx, b, row) })
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

	if isUnnamed(row.JobID) {
		// Задача не успела обрести имя: сервис либо не получил её вовсе, либо
		// получил, но сказать нам об этом не успел. Спрашивать не о чем.
		log.Printf("Задача пользователя %d умерла безымянной — спрашивать сервис не о чем\n",
			row.UserID)
		giveUpDelivery(ctx, b, row)

		return
	}

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

// handOverDelivery sends the picked-up result where it was asked from.
func handOverDelivery(ctx context.Context, b *bot.Bot, row model.PendingDelivery, data []byte) error {
	if row.InlineMessageID != "" {
		return handOverInline(ctx, b, row, data)
	}

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

// handOverInline puts the picked-up result into the inline message the button
// was pressed in.
//
// Путь тот же, что и у живой инлайн-генерации, и по той же причине: правкой
// инлайн-сообщения загрузить файл нельзя, она принимает только file_id, уже
// известный Telegram. Поэтому результат сначала кладётся в служебный чат.
//
// Подпись — исходный запрос, а не объяснение задержки: инлайн-сообщение висит
// на виду у всего чата, и ждущий текст в нём люди уже видели, так что замена
// его на результат говорит сама за себя.
func handOverInline(ctx context.Context, b *bot.Bot, row model.PendingDelivery, data []byte) error {
	if row.Kind == "video" {
		return putInlineVideo(ctx, b, row.InlineMessageID, data, row.Caption)
	}

	uploaded, err := uploadToServiceChat(ctx, func() (*models.Message, error) {
		return b.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID: os.Getenv("TMP_CHAT_ID"),
			Photo:  &models.InputFileUpload{Filename: "photo", Data: bytes.NewReader(data)},
		})
	})
	if err != nil {
		return fmt.Errorf("uploading recovered image: %w", err)
	}
	if len(uploaded.Photo) == 0 {
		return errors.New("can't get uploaded recovered image")
	}

	return replaceInlineMedia(ctx, b, row.InlineMessageID, &models.InputMediaPhoto{
		Media:      uploaded.Photo[0].FileID,
		HasSpoiler: true,
		Caption:    row.Caption,
	}, uploaded)
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
			log.Printf("[error] не вернул %d оплаченных баллов пользователю %d: %v\n",
				row.PaidUnits, row.UserID, err)
		} else {
			text = resumeRefundedText
			log.Printf("Вернул %d оплаченных баллов пользователю %d за потерянную задачу\n",
				row.PaidUnits, row.UserID)
		}
	}

	// Инлайн-сообщению сказать можно только правкой: чата, куда написать, мы
	// не знаем — Telegram его не называет.
	if row.InlineMessageID != "" {
		if err := editInlineStatus(ctx, b, row.InlineMessageID, text); err != nil {
			log.Println("[error] не сказал в инлайн-сообщение о потере")
			log.Println(err)
		}
		forgetDelivery(row.JobID)

		return
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
