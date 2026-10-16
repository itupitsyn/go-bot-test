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
// for and wraps it in the template of the chosen motion. The subject is
// translated on the way only if AI_VIDEO_TRANSLATE says so.
//
// The templates stay English even when the subject does not, and the mix is
// deliberate. Camera and motion wording ("a slow cinematic push-in", "the
// camera slowly orbits") is the vocabulary the training captions are written
// in, so it is the half that has least to gain from being translated; the
// subject is the half where Russian carries meaning no paraphrase keeps.
//
// It also reports where the prompt came from. Everything the assembly does --
// cutting the keyword, wrapping, and translating when it is on -- is lossy,
// and the raw request is worth keeping: see promptOrigin.
func buildVideoPrompt(msgText string) (string, promptOrigin, error) {
	template := getVideoTemplate(msgText)
	origin := promptOrigin{Source: msgText, Style: getVideoStyle(msgText)}

	subject := getVideoPrompt(msgText)
	if subject == "" {
		subject = defaultVideoSubject
	}

	if translateVideoPrompts() {
		translated, err := translatePrompt(subject)
		if err != nil {
			return "", origin, err
		}

		subject = translated
	}

	return applyPromptTemplate(template, subject), origin, nil
}
