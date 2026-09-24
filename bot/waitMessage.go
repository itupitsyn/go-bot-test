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
	// Wait texts in a regular chat: first "got it", at the end "working on it".
	okText   = "Ладно"
	waitText = "Жди теперь"

	// The same for an inline message in someone else's chat. The words differ
	// because there the message sits in plain view of everyone and speaks for
	// itself, with no command next to it.
	inlineOkText   = "Так, посмотрим, что у нас тут..."
	inlineWaitText = "ОЖИДАЕМ!!!"

	// summaryWaitText: a retelling has a single line, since for the asker it is
	// a short request, not a generation, so there is nothing to show but "busy"
	// and no reason to draw out a pause. Even when a voice message first waits
	// in the queue for transcription.
	summaryWaitText = "Ща, сек"

	// serverDeadText: something broke on the other side.
	serverDeadText = "Отмена, сервер подох"
	// queueFullText: the person already has as many jobs as the service takes
	// from one person at a time. Not a failure, hence the different words.
	queueFullText = "Не суетим, очередь подзабилась"
	// fileTooBigText: Telegram does not give bots files larger than 20 MB (2 GB
	// with our own bot-api). Not a failure either: retrying is useless, the
	// file will stay the same.
	fileTooBigText = "Слишком большой файл"

	// waitBeat is the pause between "Ладно" and what follows it. It is not
	// technical but comic, so it stays even when the queue is known right away.
	waitBeat = 2 * time.Second
	// statusGrace is how long in total we wait for news about the queue before
	// writing "Жди теперь" blindly. The prompt is translated by a separate llm
	// call before the job is queued, so two seconds are not always enough for
	// that, and staying silent any longer won't do: the person is already
	// waiting for an answer.
	statusGrace = 5 * time.Second
)

// waitMessage is the message under the command that lives through the whole
// generation: "Ладно", then either the place in the queue or "Жди теперь", and
// when the queue reaches our job, "Жди теперь" again.
//
// The second message used to be written on a blind timer, before the job was
// queued. Now the timer holds only the pause, and what to write is decided by
// the first known status: there is no point taking up space with "Жди теперь"
// when five more people are ahead.
type waitMessage struct {
	// messageID is what the caller edits and deletes when the generation is
	// over.
	messageID int

	waitPhrase string
	beat       time.Duration
	grace      time.Duration
	edit       func(text string) error

	firstStatus chan struct{} // closed once the queue is known
	opened      chan struct{} // closed once the opening has played
	finished    chan struct{} // closed once the message is handed to the caller
	statusOnce  sync.Once
	finishOnce  sync.Once

	mu       sync.Mutex
	status   *aiApi.QueueStatus
	lastText string
}

// newChatWaitMessage answers the command with "Ладно" and starts driving the
// message. It returns nil if the answer could not be sent: then there is
// nothing to drive.
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

// newInlineWaitMessage drives an inline message in someone else's chat.
// sendWaitInlineQueryMessage writes the opening there right when the button is
// pressed, and from then on it is the same as in a chat: a pause, then either
// the place in the queue or "ОЖИДАЕМ!!!".
//
// The pause here is exactly the same and for exactly the same reason: the
// opening and what follows are two lines in a row, and without a pause the
// second steps on the heels of the first.
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

// open plays the opening: it holds the pause, waits for news about the queue
// (but no longer than grace) and writes the second message.
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

// progress is the callback for aiApi: it reports where the job is in the queue.
func (w *waitMessage) progress(status aiApi.QueueStatus) {
	if w == nil {
		return
	}

	w.mu.Lock()
	w.status = &status
	w.mu.Unlock()

	w.statusOnce.Do(func() { close(w.firstStatus) })

	// Until the opening has played, writing to the message is its business:
	// otherwise "Ладно" would be replaced by the queue before anyone gets to
	// read it. The status is already saved; the opening will pick up the fresh
	// one.
	select {
	case <-w.opened:
		w.setText(formatQueueStatus(status, w.waitPhrase))
	default:
	}
}

// done hands the message back to the caller, who puts the result or an error
// into it or deletes it. After that the wait writes nothing more there:
// otherwise the opening would overwrite, say, "Нечего расшифровывать", which
// appears before the pause runs out.
func (w *waitMessage) done() {
	if w == nil {
		return
	}

	w.finishOnce.Do(func() { close(w.finished) })
}

// setText edits the message if the text has changed: Telegram answers a
// repeated edit with the same text with an error, and a long generation racks
// up dozens of polls.
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
		// Failing to show the wait does not hurt the generation.
		log.Println("[warn] error updating the wait message")
		log.Println(err)
	}
}

// id is the message number for the caller. Zero means there is no message:
// waitMessage methods tolerate nil so that a failed "Ладно" does not bring down
// the generation itself.
func (w *waitMessage) id() int {
	if w == nil {
		return 0
	}

	return w.messageID
}

// messageCaller is who asks for a generation in a regular chat and where to
// report the queue.
//
// A message's From is sometimes empty (for example, a post on behalf of a
// channel); then no id is sent at all, and the service files the job under the
// shared anonymous user, which is more honest than passing our zero off as a
// real id.
func messageCaller(message *models.Message, wait *waitMessage) aiApi.Caller {
	caller := aiApi.Caller{Progress: wait.progress}
	if message != nil && message.From != nil {
		caller.UserID = message.From.ID
	}

	return caller
}

// inlineCaller is the same for pressing the button under an inline result.
// Whoever presses is not necessarily the owner of the image, so we take the
// presser: the cap and the round-robin are about whoever loads the GPU.
func inlineCaller(userID int64, wait *waitMessage) aiApi.Caller {
	return aiApi.Caller{UserID: userID, Progress: wait.progress}
}
