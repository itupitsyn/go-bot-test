package aiApi

import (
	"encoding/json"
	"fmt"
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
//
// The keys are the vocabulary the llm chooses from (see videoPlanPrompt) and at
// the same time the names we group statistics by. One list, so the two cannot
// drift apart.
var videoTemplates = map[string]string{
	"slow":      "%s. The action unfolds in slow motion, fluid and unhurried, the camera holds steady. Soft ambient sound.",
	"orbit":     "%s. The camera slowly orbits the subject in one smooth continuous arc. Ambient sound moves with the camera.",
	"push-in":   "%s. A slow cinematic push-in toward the subject, the framing gradually tightening. Ambient sound grows closer.",
	"pull-back": "%s. The camera slowly pulls back to reveal the surroundings. Ambient sound opens out.",
	"aerial":    "%s. A sweeping aerial shot gliding over the scene, wind and open-air ambience.",
	"lively":    "%s. Quick confident movement, energetic action, the camera keeps up. Lively ambient sound.",
}

// defaultVideoStyleName marks a request that asked for no particular motion.
const defaultVideoStyleName = "default"

// defaultVideoTemplate applies when no motion was asked for.
const defaultVideoTemplate = "%s. Natural smooth motion, steady camera, consistent lighting. Ambient sound that matches the scene."

// defaultVideoSubject is substituted when there is no scene at all: the model
// needs at least some subject.
const defaultVideoSubject = "the scene comes to life"

// DefaultAnimationPrompt is what a picture sent without a caption is animated
// by. It is exported because the bot substitutes it and the assembly here
// recognises it: it is our own English sentence, there is nothing in it to
// plan, and a round trip to the llm only risks getting it back as "a static
// image of an unknown subject begins to move and animate" (measured
// 20.10.2026).
const DefaultAnimationPrompt = "the scene in the picture comes to life"

// A spoken line and a described scene are two different requests, and H3 has
// its own way of telling them apart. Its tokenizer carries dialogue markers:
// "<d>" and "</d>" written as plain text in the prompt become single token ids
// (151669 and 151670) -- checked against the released tokenizer in the running
// container on 19.10.2026, together with <|cutoff|>, which exists too but
// whose meaning we have not established. Whatever stands inside the markers is
// said out loud; whatever stands outside describes the shot.
//
// WHY WE NEEDED THIS. For the few days the scene went to the model in Russian
// (16.10.2026 to 20.10.2026) H3 started taking the request for the line
// itself: almost every video came out as a person facing the camera reciting
// the prompt. Which is a fair reading -- nothing in a bare phrase says whether
// it is a description or a quote. The markers say which, in the model's own
// notation, and they stay now that the scene is translated again: the line is
// still in the language it was typed in, and it is the one part that has to be
// told apart from the description.
//
// The markers cost two tokens each and no speed.
const (
	speechOpen  = "<d>"
	speechClose = "</d>"
)

// noSpeechClause is appended when there is no line. The absence of a <d> block
// by itself was not enough -- that is exactly how the reciting videos were
// made -- and a negative prompt is not available: H3 samples through
// BasicGuider, with a single conditioning and no guidance, so there is nowhere
// to put "no speech" except the prompt itself.
const noSpeechClause = "Nobody speaks and there is no voice-over: the soundtrack carries only the sounds of the scene itself."

// defaultSpeakerSubject is the scene for a request that is nothing but a line
// -- someone has to say it.
const defaultSpeakerSubject = "a person faces the camera and speaks"

// speechStyleMark is appended to the style name in statistics when the request
// carried a line. Reusing the style column instead of asking the service for a
// new one is a deliberate trade: the column is free-form text, grouping by it
// still works, and the two things we want to know ("is anyone using this" and
// "which motion goes with speech") are both answered. If speech ever becomes
// interesting on its own -- how long the lines are, which language -- it
// deserves its own column rather than more suffixes here.
const speechStyleMark = "+speech"

// modelMarkers are the model's own control tokens. They are cut out of
// everything a person typed, so that the only way into a <d> block is through
// the plan below -- a request that happens to contain "<d>" is a request about
// text, not an instruction to the model.
var modelMarkers = []string{
	speechOpen, speechClose, "<|cutoff|>",
	"<|lyrics_start|>", "<|lyrics_end|>",
	"<|caption_start|>", "<|caption_end|>",
}

// tidy squeezes the whitespace left behind by cutting markers out of the
// middle of a sentence.
func tidy(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// stripMarkers removes the model's control tokens from a text.
func stripMarkers(text string) string {
	for _, marker := range modelMarkers {
		text = strings.ReplaceAll(text, marker, " ")
	}

	return text
}

// videoPlan is what the llm makes of a request: what to show, what to say and
// how to move the camera.
type videoPlan struct {
	// Scene is the description in English. When something is said it ends with
	// the words that introduce it ("and then he says:"), which is where the
	// model looks for the quote.
	Scene string `json:"scene"`
	// Lines are the words to be said out loud, in the language they were
	// written in.
	Lines []string `json:"lines"`
	// Camera is a key of videoTemplates, or anything else for none.
	Camera string `json:"camera"`
}

// videoPlanPrompt asks the llm for that plan.
//
// WHY THE LLM AND NOT US. Until 20.10.2026 the assembly did this itself: it
// paired quotation marks to find the line, matched a keyword at the end of the
// request to pick the camera, and sent what was left to be translated. Every
// bug in the feature came from that pattern matching meeting live language --
// a keyword that stopped being last once the line was lifted out, a quote
// right after the command eaten as a separator, a scene left ending in "and
// then says:" with the line put after the camera sentence, so the model read
// our own English template aloud. Each fix was narrower than the next request.
//
// The llm is already in the path (the scene has to be translated anyway), it
// knows what a quote is in any language and what the words "slowly" and
// "langsam" ask of a camera. So it does the whole reading in one call, and the
// code keeps only what must be exact: the markers, the templates and the
// checks below.
//
// The plan comes back as data, not as a prompt. The llm never writes the
// markers itself -- otherwise a request could ask it to, and we would be
// passing a stranger's control tokens straight to the generator.
const videoPlanPrompt = `You prepare prompts for a text-to-video model.

The request may be in any language. Answer with ONE JSON object and nothing else:

{"scene": "...", "lines": ["..."], "camera": "..."}

"scene": what the camera sees, in English, translated from whatever language the
request is in. One or two sentences. Describe only what is visible. Do not add
lighting, style, sound or camera movement. Do not put the spoken words in it.
If something is said, end the scene with the words that introduce it, for
example "and then he says:".

"lines": the words to be said out loud, copied EXACTLY as written, in their
original language, without quotation marks. Keep every spelling mistake as it
is. Take them from what the user put in quotes or introduced with a verb of
speaking. Never translate them. Never invent them. If nothing is said, return [].

"camera": one of these, "none" if the request asks for no movement:
"none", "slow" (slow motion), "orbit" (the camera circles the subject),
"push-in" (the camera moves closer), "pull-back" (the camera moves away and
reveals the surroundings), "aerial" (seen from above), "lively" (quick
energetic movement).`

// planVideo asks the llm to read the request.
func planVideo(request string) (videoPlan, error) {
	answer, err := askLLM(videoPlanPrompt, request)
	if err != nil {
		return videoPlan{}, err
	}

	return parseVideoPlan(answer)
}

// parseVideoPlan reads the answer and refuses what it cannot vouch for.
func parseVideoPlan(answer string) (videoPlan, error) {
	var plan videoPlan
	if err := json.Unmarshal([]byte(unfence(answer)), &plan); err != nil {
		return videoPlan{}, fmt.Errorf("video plan is not valid json (%w): %s", err, answer)
	}

	plan.Scene = tidy(stripMarkers(plan.Scene))

	if _, ok := videoTemplates[plan.Camera]; !ok {
		// A camera we do not have a template for is not worth failing over:
		// the worst case is a video without the motion that was asked for.
		plan.Camera = defaultVideoStyleName
	}

	lines := make([]string, 0, len(plan.Lines))
	for _, line := range plan.Lines {
		line = tidy(stripMarkers(line))
		if line == "" {
			continue
		}

		// Here stood a check that every line appears in the request verbatim:
		// a line gets said out loud in somebody's video, so it had to be their
		// own words. Removed 21.10.2026, and the reason is measured rather than
		// aesthetic. On
		//
		//	встаёт со стула и уходит, говоря "Идите вы все в жопу, ребятки"
		//
		// the llm returned the line with "ребята" -- one letter -- and the
		// check failed the request whole. Upstream that arrives as an ordinary
		// error, the bot says "сервер подох", and the person loses a video over
		// a typo correction nobody would have noticed in the audio.
		//
		// What guards the words now is videoPlanPrompt, which asks for them
		// copied exactly, mistakes and all. If paraphrases or translations do
		// start showing up, the repair is cheaper than the refusal was: the
		// person's own text sits in the quotes of the request, and a line that
		// does not match can be replaced with it instead of killing the job.
		lines = append(lines, line)
	}
	plan.Lines = lines

	return plan, nil
}

// unfence unwraps an answer the model put in a markdown code fence. Asking for
// bare json is not a guarantee of getting it.
func unfence(answer string) string {
	text := strings.TrimSpace(answer)
	if !strings.HasPrefix(text, "```") {
		return text
	}

	if cut := strings.Index(text, "\n"); cut >= 0 {
		text = text[cut+1:]
	}

	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), "```"))
}

// speechBlocks wraps each line in the model's dialogue markers.
func speechBlocks(lines []string) string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, speechOpen+line+speechClose)
	}

	return strings.Join(out, " ")
}

// buildVideoPrompt assembles the final prompt out of the plan: the scene goes
// into the template of the asked-for motion, the lines go in the model's
// dialogue markers, and a request with no lines asks for silence outright.
//
// Everything the model is asked to SHOW ends up in English -- the templates
// were written that way and the scene comes back translated. The only part
// left in the language it was typed in is what the model is asked to SAY.
//
// The lines stand right next to the scene, inside the sentence the template
// wraps, and not after it. The markers alone do not decide what gets said:
// what directly follows the words that introduce speech does. Caught on
// 20.10.2026 by "человек стреляет в трусы, а потом говорит: «Я покакал»" --
// the line stood at the end of the prompt, behind the camera and sound
// sentences, and the model said our own "Natural smooth motion, steady camera,
// consistent lighting" out loud. Of everything in the prompt, that was the
// text standing where a quote belongs.
//
// It also reports where the prompt came from. The plan is a lossy reading of
// the request, and the raw words are worth keeping: see promptOrigin.
func buildVideoPrompt(msgText string) (string, promptOrigin, error) {
	request := tidy(stripMarkers(msgText))
	origin := promptOrigin{Source: msgText}

	plan := videoPlan{Scene: request, Camera: defaultVideoStyleName}
	if request != "" && request != DefaultAnimationPrompt {
		planned, err := planVideo(request)
		if err != nil {
			return "", origin, err
		}

		plan = planned
	}

	origin.Style = plan.Camera
	if len(plan.Lines) > 0 {
		origin.Style += speechStyleMark
	}

	subject := plan.Scene
	if subject == "" {
		// A request that is nothing but a line still needs someone to say it.
		subject = defaultVideoSubject
		if len(plan.Lines) > 0 {
			subject = defaultSpeakerSubject
		}
	}

	template, ok := videoTemplates[plan.Camera]
	if !ok {
		template = defaultVideoTemplate
	}

	if len(plan.Lines) > 0 {
		return applyPromptTemplate(template, subject+" "+speechBlocks(plan.Lines)), origin, nil
	}

	return applyPromptTemplate(template, subject) + " " + noSpeechClause, origin, nil
}
