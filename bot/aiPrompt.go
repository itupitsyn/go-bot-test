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

// isCommand reports whether the already lowercased text starts with one of the
// keywords used as a standalone word: the keyword is either the whole text or
// is followed by a space.
func isCommand(text string, keywords ...string) bool {
	for _, keyword := range keywords {
		rest, ok := strings.CutPrefix(text, keyword)
		if !ok {
			continue
		}
		if rest == "" {
			return true
		}
		if r, _ := utf8.DecodeRuneInString(rest); unicode.IsSpace(r) {
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
	prompt = strings.TrimSpace(prompt)

	if reply := message.ReplyToMessage; reply != nil {
		replyText := strings.TrimSpace(reply.Text)
		if replyText != "" {
			prompt = strings.TrimSpace(replyText + " " + prompt)
		}
	}

	return prompt
}
