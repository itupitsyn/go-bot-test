package aiApi

import (
	"os"
	"strings"
)

// Motion templates for video: the same thing as styles for images, except the
// useful axis here is not the look but motion and camera: in i2v the image has
// already set the look, and in t2v the request describes it anyway.
//
// They are written as sentences rather than comma-separated lists because H3
// reads the prompt with a Qwen3-VL 32B encoder and samples without guidance
// (BasicGuider, a turbo LoRA in 8 steps): "quality" tokens are no lever there,
// just extra words.
//
// Sound is mentioned in every template for a reason: H3 generates stereo in the
// same pass, and the track ends up in the mp4 anyway. Not mentioning it means
// leaving it to the model's whim.
type videoStyle struct {
	// name identifies the template in statistics. It is deliberately not the
	// suffix: suffixes are wording and may change, the name is a key we will
	// be grouping by months later.
	name string
	// suffixes are how the user turns the template on. They are looked for at
	// the end of the request, as with images: "анимируй кота вокруг".
	suffixes []string
	template string
}

// defaultVideoStyleName marks a request that asked for no template.
const defaultVideoStyleName = "default"

// defaultVideoTemplate applies when there is no keyword.
const defaultVideoTemplate = "%s. Natural smooth motion, steady camera, consistent lighting. Ambient sound that matches the scene."

// defaultVideoSubject is substituted when nothing is left after cutting off the
// keyword: the model needs at least some subject.
const defaultVideoSubject = "the scene comes to life"

// A spoken line and a described scene are two different requests, and H3 has
// its own way of telling them apart. Its tokenizer carries dialogue markers:
// "<d>" and "</d>" written as plain text in the prompt become single token ids
// (151669 and 151670) -- checked against the released tokenizer in the running
// container on 19.10.2026, together with <|cutoff|>, which exists too but
// whose meaning we have not established. Whatever stands inside the markers is
// said out loud; whatever stands outside describes the shot.
//
// WHY WE NEEDED THIS. While the subject was translated into English, the
// request read to the model as a caption and it filmed the scene. With the
// subject left in Russian (see videoTranslateEnv) H3 started taking the
// request for the line itself: almost every video came out as a person facing
// the camera reciting the prompt. Which is a fair reading -- nothing in a bare
// phrase says whether it is a description or a quote. Now the prompt says
// which, in the model's own notation.
//
// The markers cost two tokens each and no speed.
const (
	speechOpen  = "<d>"
	speechClose = "</d>"
)

// noSpeechClause is appended when nothing is quoted. The absence of a <d>
// block by itself was not enough -- that is exactly how the reciting videos
// were made -- and a negative prompt is not available: H3 samples through
// BasicGuider, with a single conditioning and no guidance, so there is nowhere
// to put "no speech" except the prompt itself.
const noSpeechClause = "Nobody speaks and there is no voice-over: the soundtrack carries only the sounds of the scene itself."

// defaultSpeakerSubject is the scene for a request that is nothing but a line
// in quotes -- someone has to say it.
const defaultSpeakerSubject = "a person faces the camera and speaks"

// speechStyleMark is appended to the style name in statistics when the request
// carried a line. Reusing the style column instead of asking the service for a
// new one is a deliberate trade: the column is free-form text, grouping by it
// still works, and the two things we want to know ("is anyone using this" and
// "which motion goes with speech") are both answered. If speech ever becomes
// interesting on its own -- how long the lines are, which language -- it
// deserves its own column rather than more suffixes here.
const speechStyleMark = "+speech"

// speechQuotes are the quotation marks a line may be written in: the ones
// people actually type on a phone. The opening mark is the key, so a straight
// double quote closes with itself.
//
// A single quote is deliberately absent: apostrophes inside words ("it's",
// "д'Артаньян") would open lines that never close.
var speechQuotes = map[rune]rune{
	'«': '»',
	'"': '"',
	'“': '”',
	'„': '“',
}

// modelMarkers are the model's own control tokens. They are stripped from
// everything a person typed, so that the only way into a <d> block is through
// the quotes above -- a request that happens to contain "<d>" is a request
// about text, not an instruction to the model.
var modelMarkers = []string{
	speechOpen, speechClose, "<|cutoff|>",
	"<|lyrics_start|>", "<|lyrics_end|>",
	"<|caption_start|>", "<|caption_end|>",
}

// tidy squeezes the whitespace left behind by cutting quotes and markers out
// of the middle of a sentence.
func tidy(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// splitSpeech takes the quoted lines out of the request and returns what is
// left of it as the scene, plus the lines in the order they were written.
//
// The lines come out BEFORE the camera keyword is looked for, and that order
// matters: cutting "«я опоздал»" out of "мужика в метро «я опоздал» медленно"
// rejoins the rest into "мужика в метро медленно", so the keyword is found at
// the end where getVideoTemplate expects it.
//
// An unpaired quote stays where it is and means nothing: people open quotes
// and forget to close them, and the alternative -- treating the whole tail as
// a line -- turns a typo into a video of someone reading it out.
func splitSpeech(text string) (string, []string) {
	runes := []rune(tidy(stripMarkers(text)))
	scene := make([]rune, 0, len(runes))
	lines := []string{}

	for i := 0; i < len(runes); i++ {
		closer, ok := speechQuotes[runes[i]]
		if !ok {
			scene = append(scene, runes[i])
			continue
		}

		end := -1
		for j := i + 1; j < len(runes); j++ {
			if runes[j] == closer {
				end = j
				break
			}
		}

		if end < 0 {
			scene = append(scene, runes[i])
			continue
		}

		if line := tidy(string(runes[i+1 : end])); line != "" {
			lines = append(lines, line)
		}

		i = end
	}

	return tidy(string(scene)), lines
}

// stripMarkers removes the model's control tokens from a person's text.
func stripMarkers(text string) string {
	for _, marker := range modelMarkers {
		text = strings.ReplaceAll(text, marker, " ")
	}

	return text
}

// speechBlocks wraps each line in the model's dialogue markers.
func speechBlocks(lines []string) string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, speechOpen+line+speechClose)
	}

	return strings.Join(out, " ")
}

// videoTranslateEnv turns translation of the video subject back on.
//
// Off since 16.10.2026: H3 reads the prompt with a Qwen3-VL 32B encoder
// (CLIPLoader of qwen3vl_32b_minimax_h3_nvfp4_awq, type minimax), and that is
// a multilingual model, so Russian can go to it as typed. What this buys is
// exactly what the translation used to cost:
//
//   - a round trip to the llm before every single video, and it was on the
//     critical path -- the person waited for it before the queue even saw the
//     request;
//   - one more thing that could fail: a translation error aborted the whole
//     generation (see buildVideoPrompt), and the llm shares a card with
//     everything else;
//   - everything a translation flattens. Names, local references and wordplay
//     come back as a generic English paraphrase, and the model then draws the
//     paraphrase.
//
// Kept as a switch rather than deleted, because the gain above is a claim we
// have not measured against our own prompts. If adherence turns out worse in
// Russian, AI_VIDEO_TRANSLATE=1 plus a restart puts it back -- no rebuild, no
// deploy.
const videoTranslateEnv = "AI_VIDEO_TRANSLATE"

// translateVideoPrompts reports whether the subject should be translated
// before it goes into the template.
//
// Only an explicit word turns it on. Absent, empty or misspelled all mean
// off, which is the behaviour we want by default: a forgotten variable must
// not quietly cost a second per video.
func translateVideoPrompts() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(videoTranslateEnv))) {
	case "1", "on", "yes", "true", "вкл":
		return true
	}

	return false
}

var videoStyles = []videoStyle{
	{
		"slow",
		[]string{" медленно", " slowly"},
		"%s. The action unfolds in slow motion, fluid and unhurried, the camera holds steady. Soft ambient sound.",
	},
	{
		"orbit",
		[]string{" вокруг", " around"},
		"%s. The camera slowly orbits the subject in one smooth continuous arc. Ambient sound moves with the camera.",
	},
	{
		"push-in",
		[]string{" ближе", " closer"},
		"%s. A slow cinematic push-in toward the subject, the framing gradually tightening. Ambient sound grows closer.",
	},
	{
		"pull-back",
		[]string{" дальше", " wider"},
		"%s. The camera slowly pulls back to reveal the surroundings. Ambient sound opens out.",
	},
	{
		"aerial",
		[]string{" сверху", " above"},
		"%s. A sweeping aerial shot gliding over the scene, wind and open-air ambience.",
	},
	{
		"lively",
		[]string{" живо", " lively"},
		"%s. Quick confident movement, energetic action, the camera keeps up. Lively ambient sound.",
	},
}

// cutVideoSuffix reports whether the request ends in the given keyword and
// gives back what is left before it. All three readers of the table below go
// through it, so "ends in a keyword" cannot come to mean three slightly
// different things.
//
// Case is compared fold-wise instead of lowercasing the whole string, and what
// is left is cut out of the ORIGINAL text, so the subject keeps the case the
// person typed. While the subject was translated this made no difference --
// the llm restored the case on its own -- but now it reaches the encoder as
// typed, and "Красная площадь" is not the same thing to it as "красная
// площадь".
//
// The tail is measured in runes, not bytes: the keywords are Russian, one
// letter there is two bytes, and byte arithmetic over a lowercased copy would
// cut a word in half on the first character that changes length in lower case.
func cutVideoSuffix(text, suffix string) (string, bool) {
	runes, tail := []rune(text), []rune(suffix)
	if len(runes) < len(tail) {
		return text, false
	}

	border := len(runes) - len(tail)
	if !strings.EqualFold(string(runes[border:]), suffix) {
		return text, false
	}

	return strings.TrimSpace(string(runes[:border])), true
}

// getVideoStyle names the template the trailing keyword asks for. Reads the
// same table as getVideoTemplate so the two cannot drift apart.
func getVideoStyle(msgText string) string {
	text := strings.TrimSpace(msgText)

	for _, style := range videoStyles {
		for _, suffix := range style.suffixes {
			if _, ok := cutVideoSuffix(text, suffix); ok {
				return style.name
			}
		}
	}

	return defaultVideoStyleName
}

// getVideoTemplate returns the template the trailing keyword asks for, or the
// default one when the request ends in no keyword at all.
func getVideoTemplate(msgText string) string {
	text := strings.TrimSpace(msgText)

	for _, style := range videoStyles {
		for _, suffix := range style.suffixes {
			if _, ok := cutVideoSuffix(text, suffix); ok {
				return style.template
			}
		}
	}

	return defaultVideoTemplate
}

// getVideoPrompt cuts the trailing keyword off, leaving what the video is
// actually about. The template says the rest.
//
// The case is left as the person typed it -- see cutVideoSuffix for why that
// started to matter.
func getVideoPrompt(msgText string) string {
	text := strings.TrimSpace(msgText)

	for _, style := range videoStyles {
		for _, suffix := range style.suffixes {
			if cut, ok := cutVideoSuffix(text, suffix); ok {
				return cut
			}
		}
	}

	return text
}

// buildVideoPrompt assembles the final prompt: it takes what the person asked
// for and wraps it in the template of the chosen motion. Anything they put in
// quotes becomes a spoken line instead of part of the description, and what is
// left is the scene. The subject is translated on the way only if
// AI_VIDEO_TRANSLATE says so.
//
// The templates stay English even when the subject does not, and the mix is
// deliberate. Camera and motion wording ("a slow cinematic push-in", "the
// camera slowly orbits") is the vocabulary the training captions are written
// in, so it is the half that has least to gain from being translated; the
// subject is the half where Russian carries meaning no paraphrase keeps.
//
// It also reports where the prompt came from. Everything the assembly does --
// cutting the keyword, lifting the lines out, wrapping, and translating when
// it is on -- is lossy, and the raw request is worth keeping: see promptOrigin.
func buildVideoPrompt(msgText string) (string, promptOrigin, error) {
	scene, lines := splitSpeech(msgText)

	template := getVideoTemplate(scene)
	origin := promptOrigin{Source: msgText, Style: getVideoStyle(scene)}
	if len(lines) > 0 {
		origin.Style += speechStyleMark
	}

	subject := getVideoPrompt(scene)
	if subject == "" {
		// A request that is nothing but a line still needs someone to say it.
		subject = defaultVideoSubject
		if len(lines) > 0 {
			subject = defaultSpeakerSubject
		}
	}

	if translateVideoPrompts() {
		translated, err := translatePrompt(subject)
		if err != nil {
			return "", origin, err
		}

		subject = translated
	}

	prompt := applyPromptTemplate(template, subject)

	// The lines themselves are never translated, not even with translation
	// turned back on: they are what the person will hear said, and the whole
	// point of a line is that it is said in the words it was written in.
	if len(lines) > 0 {
		return prompt + " " + speechBlocks(lines), origin, nil
	}

	return prompt + " " + noSpeechClause, origin, nil
}
