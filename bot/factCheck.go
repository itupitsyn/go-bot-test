package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"telebot/aiApi"
	"telebot/utils"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	// A check now reads the message, searches two engines, sometimes twice, and
	// takes about 40 s: the wait text says so.
	factCheckWaitText = "Ща, проверю. Это может занять до минуты"
	// factNothingText answers a message with nothing to check when no joke came
	// out of the model.
	factNothingText = "Тут проверять нечего."
	// factNoTextText: neither the message nor its pictures have words to check.
	factNoTextText = "Текста тут не вижу, проверять нечего."
	// factPictureLabel heads the text read off a picture in the claim.
	factPictureLabel = "Текст на картинке"
	// factPrimaryLabel marks the source the model took for the closest to the
	// original.
	factPrimaryLabel = "Первоисточник: "
	// factJokeMemory is how many of its last jokes per chat the bot shows the
	// model, so the same trick does not come round again and again.
	factJokeMemory = 5

	// The two search failures read differently on purpose: one is our own
	// server lying down, the other is the engines refusing us (a captcha, a
	// ban). Neither may come out as "not found": nothing was searched.
	searchDownText  = "Поиск сейчас лежит, проверить не могу"
	enginesDownText = "Поисковики сейчас не отвечают, проверить не могу"
	// searchBusyText: the shared queue to the search is over five minutes long.
	// Not a failure, so it does not say anything died.
	searchBusyText = "Поиск сейчас занят другими проверками, попробуй попозже"

	factConfirmedText   = "Подтверждается."
	factRefutedText     = "Опровергается."
	factConflictingText = "Источники расходятся."
	// "Not found" is the most frequent honest answer and the easiest to
	// misread, hence the second sentence.
	factNotFoundText = "Подтверждений не нашёл. Это ещё не значит, что это неправда."

	// factCheckCacheTTL: the same claim asked again within it gets the same
	// answer without a new search. News moves, so not for long.
	factCheckCacheTTL  = time.Hour
	factCheckCacheSize = 200
	// factCheckPerHour is per person. Engines start answering captchas to a
	// burst, and that hurts every chat at once.
	factCheckPerHour = 10
	// maxEvidenceShown and maxQuoteRunes keep the answer short.
	maxEvidenceShown = 3
	maxQuoteRunes    = 220
)

func isFactCheckCommand(text string) bool {
	return isBareCommand(text, "проверь", "fact check", "factcheck")
}

type factCacheEntry struct {
	result *aiApi.FactCheck
	at     time.Time
}

// factCheckGate holds the cache and the per-person budget. In memory only: a
// restart forgets both, which is fine for a few checks a day.
type factCheckGate struct {
	mu      sync.Mutex
	cache   map[string]factCacheEntry
	asked   map[int64][]time.Time
	running map[int64]bool
}

var factGate = &factCheckGate{
	cache:   map[string]factCacheEntry{},
	asked:   map[int64][]time.Time{},
	running: map[int64]bool{},
}

// factJokeLog keeps each chat's last jokes. In memory only: after a restart the
// first jokes may repeat a trick from before it, which is harmless.
type factJokeLog struct {
	mu     sync.Mutex
	byChat map[int64][]string
}

var factJokes = &factJokeLog{byChat: map[int64][]string{}}

func (l *factJokeLog) recent(chatID int64) []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return append([]string(nil), l.byChat[chatID]...)
}

func (l *factJokeLog) remember(chatID int64, joke string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	jokes := append(l.byChat[chatID], joke)
	if len(jokes) > factJokeMemory {
		jokes = jokes[len(jokes)-factJokeMemory:]
	}
	l.byChat[chatID] = jokes
}

func factCacheKey(claim string) string {
	return strings.Join(strings.Fields(strings.ToLower(claim)), " ")
}

func (g *factCheckGate) cached(claim string, now time.Time) *aiApi.FactCheck {
	g.mu.Lock()
	defer g.mu.Unlock()

	entry, ok := g.cache[factCacheKey(claim)]
	if !ok || now.Sub(entry.at) > factCheckCacheTTL {
		return nil
	}

	return entry.result
}

func (g *factCheckGate) remember(claim string, result *aiApi.FactCheck, now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.cache) >= factCheckCacheSize {
		for key, entry := range g.cache {
			if now.Sub(entry.at) > factCheckCacheTTL {
				delete(g.cache, key)
			}
		}
	}
	if len(g.cache) >= factCheckCacheSize {
		oldestKey, oldestAt := "", now
		for key, entry := range g.cache {
			if entry.at.Before(oldestAt) {
				oldestKey, oldestAt = key, entry.at
			}
		}
		delete(g.cache, oldestKey)
	}
	g.cache[factCacheKey(claim)] = factCacheEntry{result: result, at: now}
}

// admit takes one check out of the person's hourly budget. A person with a
// check still running is refused too: a second one would only queue behind it.
func (g *factCheckGate) admit(userID int64, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.running[userID] {
		return false
	}

	recent := g.asked[userID][:0]
	for _, at := range g.asked[userID] {
		if now.Sub(at) < time.Hour {
			recent = append(recent, at)
		}
	}
	if len(recent) >= factCheckPerHour {
		g.asked[userID] = recent
		return false
	}

	g.asked[userID] = append(recent, now)
	g.running[userID] = true

	return true
}

func (g *factCheckGate) done(userID int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.running, userID)
}

func clipQuote(quote string) string {
	runes := []rune(strings.TrimSpace(quote))
	if len(runes) <= maxQuoteRunes {
		return string(runes)
	}

	return strings.TrimSpace(string(runes[:maxQuoteRunes])) + "…"
}

// formatFactCheck is the answer: the verdict, then the phrases it rests on with
// their links, so anyone can see for themselves what the bot read.
func formatFactCheck(result *aiApi.FactCheck) string {
	head := map[aiApi.FactVerdict]string{
		aiApi.FactConfirmed:   factConfirmedText,
		aiApi.FactRefuted:     factRefutedText,
		aiApi.FactConflicting: factConflictingText,
		aiApi.FactNotFound:    factNotFoundText,
	}[result.Verdict]
	if head == "" {
		head = factNotFoundText
	}

	lines := []string{head}
	if result.Primary != nil && result.Primary.URL != "" {
		lines = append(lines, factPrimaryLabel+result.Primary.URL)
	}
	for i, item := range result.Evidence {
		if i == maxEvidenceShown {
			break
		}
		side := ""
		if result.Verdict == aiApi.FactConflicting {
			side = "Против: "
			if item.Supports {
				side = "За: "
			}
		}
		lines = append(lines, fmt.Sprintf("\n%s«%s»\n%s", side, clipQuote(item.Quote), item.URL))
	}

	return strings.Join(lines, "\n")
}

// factCheckFailText turns an error into words. The search failures are told
// apart from each other and from everything else.
func factCheckFailText(err error) string {
	switch {
	case errors.Is(err, aiApi.ErrSearchBusy):
		return searchBusyText
	case errors.Is(err, aiApi.ErrSearchEnginesDown):
		return enginesDownText
	case errors.Is(err, aiApi.ErrSearchUnavailable):
		return searchDownText
	default:
		return serverDeadText
	}
}

// joinPictureText puts the text of a message and the text read off its
// pictures into one claim, so the check sees everything that was said.
func joinPictureText(text string, pictures []string) string {
	parts := []string{}
	if text = strings.TrimSpace(text); text != "" {
		parts = append(parts, text)
	}
	for i, picture := range pictures {
		label := factPictureLabel
		if len(pictures) > 1 {
			label = fmt.Sprintf("%s %d", factPictureLabel, i+1)
		}
		parts = append(parts, label+":\n"+picture)
	}
	return strings.Join(parts, "\n\n")
}

// readFactPictures reads the text off each picture. Pictures without text are
// left out; a picture that failed is skipped as long as another one was read.
// The second value is the text to show when nothing could be read at all.
func readFactPictures(ctx context.Context, b *bot.Bot, images []string) ([]string, string) {
	texts := []string{}
	failed, failText := 0, ""
	for _, id := range images {
		image, _, err := downloadTelegramFile(ctx, b, id)
		if err != nil {
			log.Println("[error] fact check picture download:", err)
			failed++
			failText = serverDeadText
			if errors.Is(err, ErrFileTooBig) {
				failText = fileTooBigText
			}
			continue
		}
		text, err := aiApi.ReadPictureText(image)
		if err != nil {
			log.Println("[error] fact check picture read:", err)
			failed++
			failText = serverDeadText
			continue
		}
		if text != "" {
			texts = append(texts, text)
		}
	}
	log.Println("Fact check pictures:", len(images), "with text:", len(texts), "failed:", failed)

	if failed == len(images) {
		return nil, failText
	}
	return texts, ""
}

// processFactCheck checks the message the command replies to against a web
// search, together with the text on its pictures. A picture with "проверь" as
// its caption is checked too. Like "что тут", a bare "проверь" replying to
// nothing is chatter and gets no answer.
func processFactCheck(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := update.Message.Chat.ID
	now := time.Now()

	source := update.Message.ReplyToMessage
	text := ""
	if source == nil {
		if getDescribableImageFileID(update.Message) == "" {
			return
		}
		// The caption is the command itself, so only the pictures count.
		source = update.Message
	} else {
		text = strings.TrimSpace(getMessageText(source))
		if text == "" {
			// A reply to a picture of an album: the caption is on another one.
			text = strings.TrimSpace(albumCaptions.lookup(source, now))
		}
		if isFactCheckCommand(strings.ToLower(text)) {
			text = ""
		}
	}
	images := factImages(source, now)

	reply := func(text string, replyToId int) {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:             chatId,
			Text:               text,
			ReplyParameters:    &models.ReplyParameters{MessageID: replyToId},
			LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()},
		})
		utils.ProcessSendMessageError(err, chatId)
	}

	if text == "" && len(images) == 0 {
		// A video or a file without words still deserves an answer: silence
		// looks like the bot is broken.
		if hasMedia(source) {
			reply(factNoTextText, update.Message.ID)
		}
		return
	}

	if replyIfMaintenance(ctx, b, update.Message) {
		return
	}

	var userID int64
	if update.Message.From != nil {
		userID = update.Message.From.ID
	}
	admitted := false
	admit := func() bool {
		if !factGate.admit(userID, now) {
			reply(aiLimitFallbackText, update.Message.ID)
			return false
		}
		admitted = true
		return true
	}
	defer func() {
		if admitted {
			factGate.done(userID)
		}
	}()

	var waitMsg *models.Message
	sendWait := func() {
		msg, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatId,
			Text:            factCheckWaitText,
			ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
		})
		utils.ProcessSendMessageError(err, chatId)
		waitMsg = msg
	}
	// fail puts the text in place of the wait message.
	fail := func(text string) {
		if waitMsg == nil {
			reply(text, update.Message.ID)
			return
		}
		_, botError := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatId,
			MessageID: waitMsg.ID,
			Text:      text,
		})
		utils.ProcessSendMessageError(botError, chatId)
	}

	claim := text
	if len(images) > 0 {
		// Reading pictures costs llm calls, so it goes under the same limit as
		// the check itself, before the cache can be asked.
		if !admit() {
			return
		}
		sendWait()
		pictures, failText := readFactPictures(ctx, b, images)
		if failText != "" && text == "" {
			fail(failText)
			return
		}
		claim = joinPictureText(text, pictures)
		if claim == "" {
			fail(factNoTextText)
			return
		}
	}

	result := factGate.cached(claim, now)
	if result == nil {
		if !admitted {
			if !admit() {
				return
			}
			sendWait()
		}

		var err error
		result, err = aiApi.CheckFact(claim)
		if err != nil {
			log.Println("[error] fact check failed")
			log.Println(err)
			fail(factCheckFailText(err))
			return
		}
		factGate.remember(claim, result, now)
	}
	if waitMsg != nil {
		b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatId, MessageID: waitMsg.ID})
	}

	if result.Verdict == aiApi.FactNotCheckable {
		joke, err := aiApi.GenerateFactJoke(claim, factJokes.recent(chatId))
		if err != nil {
			log.Println("[error] fact check joke:", err)
		}
		if joke == "" {
			joke = factNothingText
		} else {
			factJokes.remember(chatId, joke)
		}
		log.Println("Fact check verdict:", result.Verdict)
		reply(joke, source.ID)
		return
	}

	log.Println("Fact check verdict:", result.Verdict, "evidence:", len(result.Evidence), "primary:", result.Primary != nil)
	reply(formatFactCheck(result), source.ID)
}
