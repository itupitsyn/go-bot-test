package bot

import (
	"strings"
	"telebot/utils"
	"unicode"
	"unicode/utf8"

	"github.com/go-telegram/bot/models"
)

// defaultAnimationPrompt is used when a picture is handed to the animator
// without any words of its own. It only names the subject: the motion and the
// sound come from the video template around it.
const defaultAnimationPrompt = "the scene in the picture comes to life"

// quotationMarks lists the marks that ask the video model for a spoken line
// (speechQuotes in the aiApi package).
const quotationMarks = `«»"“”„`

// isCommandSeparator reports whether the rune ends the keyword. It is not just
// a space: "анимируй, котика" and "нарисуй: котика" are commands too, people
// type them with a comma or a colon without thinking.
func isCommandSeparator(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
}

// isCommand reports whether the already lowercased text starts with one of the
// keywords used as a standalone word: the keyword is either the whole text or
// is followed by a space or a punctuation mark. A letter right after it makes
// it a different word: "нарисуйте" does not count as a command.
func isCommand(text string, keywords ...string) bool {
	for _, keyword := range keywords {
		rest, ok := strings.CutPrefix(text, keyword)
		if !ok {
			continue
		}
		if rest == "" {
			return true
		}
		if r, _ := utf8.DecodeRuneInString(rest); isCommandSeparator(r) {
			return true
		}
	}

	return false
}

// isPromptBorder reports whether the rune did nothing but separate the command
// from the prompt and can be dropped along with the keyword. A quotation mark
// does more than separate: it marks the words to be spoken aloud, so "анимируй
// «привет, это я»" has to keep it and ask for a line, not animate the words.
func isPromptBorder(r rune) bool {
	return isCommandSeparator(r) && !strings.ContainsRune(quotationMarks, r)
}

// getMessageText returns the text of a message, falling back to the caption of
// a media message.
func getMessageText(message *models.Message) string {
	if message.Text != "" {
		return message.Text
	}

	return message.Caption
}

// buildAiPrompt builds a generation prompt out of a command message: the
// command keyword is cut off and, when the command replies to a text message,
// the text of that message becomes the beginning of the prompt. So "нарисуй
// аниме" sent in reply to "котик на подоконнике" gives the prompt "котик на
// подоконнике аниме".
func buildAiPrompt(message *models.Message, keywords ...string) string {
	return joinPrompt(replyPrompt(message), commandPrompt(message, keywords...))
}

// buildAnimationPrompt does the same for the animator, with one difference: the
// text being replied to goes in as a description of the scene and nothing else.
// Quotation marks in it are an ordinary quotation — it is somebody else's
// message, and whoever wrote it was not asking for words to be said out loud.
// Asking for a line stays with the person typing the command.
func buildAnimationPrompt(message *models.Message, keywords ...string) string {
	scene := dropQuotationMarks(replyPrompt(message))

	return joinPrompt(scene, commandPrompt(message, keywords...))
}

// commandPrompt returns what is left of the command message once the keyword is
// cut off.
func commandPrompt(message *models.Message, keywords ...string) string {
	prompt := getMessageText(message)
	for _, keyword := range keywords {
		prompt = utils.TrimPrefixIgnoreCase(prompt, keyword)
	}
	// The punctuation mark that separated the prompt is not needed in the
	// prompt itself: "анимируй, котика" must send "котика" to the service, not
	// ", котика".
	prompt = strings.TrimLeftFunc(prompt, isPromptBorder)

	return strings.TrimSpace(prompt)
}

// replyPrompt returns the text of the message being replied to. The caption of
// a media message is not taken: a reply to a photo is about the photo, and the
// caption under it is usually a remark of its own.
func replyPrompt(message *models.Message) string {
	if reply := message.ReplyToMessage; reply != nil {
		return strings.TrimSpace(reply.Text)
	}

	return ""
}

// joinPrompt puts the two halves of the prompt together: the text being replied
// to first, what was typed with the command after it.
func joinPrompt(reply, command string) string {
	if reply == "" {
		return command
	}

	return strings.TrimSpace(reply + " " + command)
}

// dropQuotationMarks takes the quotation marks out of the text, leaving the
// words themselves in place. A space takes the place of every mark so that
// nothing sticks together.
func dropQuotationMarks(text string) string {
	cleaned := strings.Map(func(r rune) rune {
		if strings.ContainsRune(quotationMarks, r) {
			return ' '
		}

		return r
	}, text)

	return strings.Join(strings.Fields(cleaned), " ")
}

// isBareCommand reports whether the already lowercased text is nothing but one
// of the keywords, optionally followed by question marks. Unlike isCommand it
// takes no arguments after the keyword: "что тут" and "что тут?" are commands,
// while "что тут происходит" is ordinary chatter the bot must stay out of.
func isBareCommand(text string, keywords ...string) bool {
	text = strings.TrimSpace(text)
	for _, keyword := range keywords {
		rest, ok := strings.CutPrefix(text, keyword)
		if !ok {
			continue
		}
		if strings.TrimLeft(rest, " ?") == "" {
			return true
		}
	}

	return false
}
