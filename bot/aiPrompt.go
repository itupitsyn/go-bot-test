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
	prompt := getMessageText(message)
	for _, keyword := range keywords {
		prompt = utils.TrimPrefixIgnoreCase(prompt, keyword)
	}
	// The punctuation mark that separated the prompt is not needed in the
	// prompt itself: "анимируй, котика" must send "котика" to the service, not
	// ", котика".
	prompt = strings.TrimLeftFunc(prompt, isCommandSeparator)
	prompt = strings.TrimSpace(prompt)

	if reply := message.ReplyToMessage; reply != nil {
		replyText := strings.TrimSpace(reply.Text)
		if replyText != "" {
			prompt = strings.TrimSpace(replyText + " " + prompt)
		}
	}

	return prompt
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
