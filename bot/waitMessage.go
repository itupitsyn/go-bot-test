package bot

import (
	"context"
	"log"
	"sync"
	"telebot/aiApi"
	"telebot/utils"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	// Тексты ожидания в обычном чате: сначала «принял», в конце «считаю».
	okText   = "Ладно"
	waitText = "Жди теперь"

	// То же самое для inline-сообщения в чужом чате. Слова другие, потому что
	// там сообщение стоит у всех на виду и говорит само за себя, без команды
	// рядом.
	inlineOkText   = "Так, посмотрим, что у нас тут..."
	inlineWaitText = "ОЖИДАЕМ!!!"

	// summaryWaitText — у пересказа реплика одна: он идёт мимо очереди
	// генерации, отдельным llm-вызовом, так что показывать там нечего, кроме
	// «занят», и тянуть паузу не за чем.
	summaryWaitText = "Ща, сек"

	// serverDeadText — что-то сломалось на той стороне.
	serverDeadText = "Отмена, сервер подох"
	// queueFullText — у человека уже столько задач, сколько сервис берёт
	// с одного разом. Не поломка, поэтому и слова другие.
	queueFullText = "Не суетим, очередь подзабилась"
	// fileTooBigText — Telegram не отдаёт боту файлы больше 20 МБ. Тоже не
	// поломка: повторять бесполезно, файл таким и останется.
	fileTooBigText = "Слишком большой файл"

	// waitBeat — пауза между «Ладно» и тем, что за ним. Она тут не
	// техническая, а комическая, поэтому остаётся, даже когда про очередь
	// известно сразу.
	waitBeat = 2 * time.Second
	// statusGrace — сколько всего ждём вестей об очереди, прежде чем написать
	// «Жди теперь» вслепую. Промпт перед постановкой в очередь переводит
	// отдельный llm-вызов, так что двух секунд на это не всегда хватает,
	// а молчать дольше нельзя: человек уже ждёт ответа.
	statusGrace = 5 * time.Second
)

// waitMessage — сообщение под командой, живущее всю генерацию: «Ладно», потом
// либо место в очереди, либо «Жди теперь», а когда очередь дойдёт до нашей
// задачи — снова «Жди теперь».
//
// Раньше второе сообщение писалось по таймеру вслепую, до постановки задачи в
// очередь. Теперь таймер держит только паузу, а что писать — решает первый
// известный статус: занимать место под «Жди теперь», когда впереди ещё
// пятеро, незачем.
type waitMessage struct {
	// messageID — что правит и удаляет вызывающий, когда генерация кончилась.
	messageID int

	waitPhrase string
	beat       time.Duration
	grace      time.Duration
	edit       func(text string) error

	firstStatus chan struct{} // закрывается, когда очередь стала известна
	opened      chan struct{} // закрывается, когда вступление отыграно
	finished    chan struct{} // закрывается, когда сообщение отдали вызывающему
	statusOnce  sync.Once
	finishOnce  sync.Once

	mu       sync.Mutex
	status   *aiApi.QueueStatus
	lastText string
}

// newChatWaitMessage отвечает «Ладно» на команду и начинает вести сообщение.
// Вернёт nil, если ответить не удалось: тогда вести нечего.
func newChatWaitMessage(ctx context.Context, b *bot.Bot, chatId int64, replyToMessageId int) *waitMessage {
	msg, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatId,
		Text:            okText,
		ReplyParameters: &models.ReplyParameters{MessageID: replyToMessageId, ChatID: chatId},
	})
	utils.ProcessSendMessageError(err, chatId)
	if err != nil || msg == nil {
		return nil
	}

	w := newWaitMessage(msg.ID, waitText, okText, func(text string) error {
		_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatId,
			MessageID: msg.ID,
			Text:      text,
		})
		return err
	})

	go w.open()

	return w
}

// newInlineWaitMessage ведёт inline-сообщение в чужом чате. Вступление туда
// пишет sendWaitInlineQueryMessage сразу по нажатию кнопки, а дальше всё как в
// чате: пауза, потом место в очереди либо «ОЖИДАЕМ!!!».
//
// Пауза тут ровно та же и ровно за тем же: вступление и то, что за ним, — это
// две реплики подряд, и без паузы вторая наступает первой на пятки.
func newInlineWaitMessage(ctx context.Context, b *bot.Bot, inlineMessageId string) *waitMessage {
	w := newWaitMessage(0, inlineWaitText, inlineOkText, func(text string) error {
		return editInlineStatus(ctx, b, inlineMessageId, text)
	})

	go w.open()

	return w
}

func newWaitMessage(messageID int, waitPhrase, currentText string, edit func(text string) error) *waitMessage {
	return &waitMessage{
		messageID:   messageID,
		waitPhrase:  waitPhrase,
		beat:        waitBeat,
		grace:       statusGrace,
		edit:        edit,
		firstStatus: make(chan struct{}),
		opened:      make(chan struct{}),
		finished:    make(chan struct{}),
		lastText:    currentText,
	}
}

// open отыгрывает вступление: держит паузу, дожидается вестей об очереди — но
// не дольше grace — и пишет второе сообщение.
func (w *waitMessage) open() {
	grace := time.After(w.grace)

	select {
	case <-time.After(w.beat):
	case <-w.finished:
		return
	}

	select {
	case <-w.firstStatus:
	case <-grace:
	case <-w.finished:
		return
	}

	w.mu.Lock()
	status := w.status
	w.mu.Unlock()

	text := w.waitPhrase
	if status != nil {
		text = formatQueueStatus(*status, w.waitPhrase)
	}
	w.setText(text)

	close(w.opened)
}

// progress — колбэк для aiApi: сообщает, где задача в очереди.
func (w *waitMessage) progress(status aiApi.QueueStatus) {
	if w == nil {
		return
	}

	w.mu.Lock()
	w.status = &status
	w.mu.Unlock()

	w.statusOnce.Do(func() { close(w.firstStatus) })

	// Пока вступление не отыграло, писать в сообщение — его дело: иначе
	// «Ладно» сменится очередью раньше, чем его успеют прочитать. Статус мы
	// уже сохранили, вступление возьмёт свежий.
	select {
	case <-w.opened:
		w.setText(formatQueueStatus(status, w.waitPhrase))
	default:
	}
}

// done возвращает сообщение вызывающему: тот кладёт в него результат, ошибку
// или удаляет его. После этого ожидание туда больше не пишет — иначе
// вступление затрёт, например, «Нечего расшифровывать», которое появляется
// раньше, чем истечёт пауза.
func (w *waitMessage) done() {
	if w == nil {
		return
	}

	w.finishOnce.Do(func() { close(w.finished) })
}

// setText правит сообщение, если текст изменился: Telegram на повторной
// правке тем же текстом отвечает ошибкой, а опросов за долгую генерацию
// набегают десятки.
func (w *waitMessage) setText(text string) {
	select {
	case <-w.finished:
		return
	default:
	}

	w.mu.Lock()
	if text == w.lastText {
		w.mu.Unlock()
		return
	}
	w.lastText = text
	w.mu.Unlock()

	if err := w.edit(text); err != nil {
		// Не показали ожидание — генерация от этого не страдает.
		log.Println("[warn] error updating the wait message")
		log.Println(err)
	}
}

// id — номер сообщения для вызывающего. Ноль означает, что сообщения нет:
// методы waitMessage терпят nil, чтобы неудачная отправка «Ладно» не роняла
// саму генерацию.
func (w *waitMessage) id() int {
	if w == nil {
		return 0
	}

	return w.messageID
}

// messageCaller — кто просит генерацию в обычном чате и куда сообщать про
// очередь.
//
// From у сообщения бывает пустым (например, пост от имени канала); тогда id не
// шлём вовсе, и сервис отнесёт задачу к общему анонимному пользователю — это
// честнее, чем выдавать наш ноль за настоящий id.
func messageCaller(message *models.Message, wait *waitMessage) aiApi.Caller {
	caller := aiApi.Caller{Progress: wait.progress}
	if message != nil && message.From != nil {
		caller.UserID = message.From.ID
	}

	return caller
}

// inlineCaller — то же для нажатия кнопки под inline-результатом. Жмёт не
// обязательно тот, чья там картинка, поэтому берём именно нажавшего: потолок
// и круг — про того, кто грузит карту.
func inlineCaller(userID int64, wait *waitMessage) aiApi.Caller {
	return aiApi.Caller{UserID: userID, Progress: wait.progress}
}
