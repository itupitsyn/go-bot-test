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
	factCheckWaitText = "Ща, проверю"

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

// processFactCheck checks the message the command replies to against a web
// search. Like "что тут", a bare "проверь" replying to nothing is chatter and
// gets no answer.
func processFactCheck(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := update.Message.Chat.ID

	source := update.Message.ReplyToMessage
	if source == nil {
		return
	}
	claim := strings.TrimSpace(getMessageText(source))
	if claim == "" {
		return
	}

	if replyIfMaintenance(ctx, b, update.Message) {
		return
	}

	reply := func(text string, replyToId int) {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:             chatId,
			Text:               text,
			ReplyParameters:    &models.ReplyParameters{MessageID: replyToId},
			LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()},
		})
		utils.ProcessSendMessageError(err, chatId)
	}

	now := time.Now()
	result := factGate.cached(claim, now)
	if result == nil {
		var userID int64
		if update.Message.From != nil {
			userID = update.Message.From.ID
		}
		if !factGate.admit(userID, now) {
			reply(aiLimitFallbackText, update.Message.ID)
			return
		}
		defer factGate.done(userID)

		waitMsg, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatId,
			Text:            factCheckWaitText,
			ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
		})
		utils.ProcessSendMessageError(err, chatId)

		result, err = aiApi.CheckFact(claim)
		if err != nil {
			log.Println("[error] fact check failed")
			log.Println(err)
			text := factCheckFailText(err)
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
			return
		}
		factGate.remember(claim, result, now)

		if waitMsg != nil {
			b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatId, MessageID: waitMsg.ID})
		}
	}

	log.Println("Fact check verdict:", result.Verdict, "evidence:", len(result.Evidence))
	reply(formatFactCheck(result), source.ID)
}
