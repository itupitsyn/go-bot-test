package aiApi

import "strings"

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

// getVideoStyle names the template the trailing keyword asks for. Reads the
// same table as getVideoTemplate so the two cannot drift apart.
func getVideoStyle(msgText string) string {
	text := strings.ToLower(strings.TrimSpace(msgText))

	for _, style := range videoStyles {
		for _, suffix := range style.suffixes {
			if strings.HasSuffix(text, suffix) {
				return style.name
			}
		}
	}

	return defaultVideoStyleName
}

// getVideoTemplate returns the template the trailing keyword asks for, or the
// default one when the request ends in no keyword at all.
func getVideoTemplate(msgText string) string {
	text := strings.ToLower(strings.TrimSpace(msgText))

	for _, style := range videoStyles {
		for _, suffix := range style.suffixes {
			if strings.HasSuffix(text, suffix) {
				return style.template
			}
		}
	}

	return defaultVideoTemplate
}

// getVideoPrompt cuts the trailing keyword off, leaving what the video is
// actually about. The template says the rest.
func getVideoPrompt(msgText string) string {
	text := strings.ToLower(strings.TrimSpace(msgText))

	for _, style := range videoStyles {
		for _, suffix := range style.suffixes {
			if cut, ok := strings.CutSuffix(text, suffix); ok {
				return strings.TrimSpace(cut)
			}
		}
	}

	return text
}

// buildVideoPrompt assembles the final prompt: it translates what the user
// asked for and wraps it in the template of the chosen motion.
//
// It also reports where that prompt came from. Everything the assembly does
// -- cutting the keyword, lowercasing, translating, wrapping -- is lossy, and
// the raw request is worth keeping: see promptOrigin.
func buildVideoPrompt(msgText string) (string, promptOrigin, error) {
	template := getVideoTemplate(msgText)
	origin := promptOrigin{Source: msgText, Style: getVideoStyle(msgText)}

	subject := getVideoPrompt(msgText)
	if subject == "" {
		subject = defaultVideoSubject
	}

	translated, err := translatePrompt(subject)
	if err != nil {
		return "", origin, err
	}

	return applyPromptTemplate(template, translated), origin, nil
}
