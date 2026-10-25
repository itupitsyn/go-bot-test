package aiApi

import (
	"log"
	"regexp"
	"strings"
)

// Prompt enhancement replaced the style templates on 25.10.2026.
//
// WHY THE TEMPLATES WENT. The styles were opt-in by a trailing keyword
// (" anime", " аниме", " cyberpunk" and so on). The service journal says the
// keyword was never once guessed for a picture: of 37 image jobs 22 carried a
// style name and all 22 were "plain", and 40 edit jobs carried none at all.
// Video styles stay — there people do reach for them, "pull-back" alone shows
// up in 48 of 162 clips.
//
// WHY ENHANCEMENT INSTEAD. The same journal shows picture requests are
// telegraphic: the median is 5 words, 17 of 22 fit into ten, and only 2 run
// past twenty-five. A model draws exactly what those five words say and
// nothing more — the "wow" people expect lives in the words that were never
// written, not in a bigger model. Measured on 25.10.2026: the same subject,
// the same model and the same seed, with the idea expanded into a frame, is
// not comparable to the bare request.
//
// The expansion is one call to the same llm the translator already uses, so
// no new service and no new card: it costs about 2.5 seconds and translates
// on the way, which the bare translation used to do anyway.
//
// WHY 40-60 WORDS AND NOT 400. Qwen publish their own rewriter for this very
// model (Qwen-Image-2.1-PE-T2I) and its system prompt asks for 400-500 words
// in a strict eight-step layout. We measured both on live requests, one seed
// each: the long style lost. "кота" turned into a flat catalogue portrait on
// white, and "Отмена, сервер подох" was read as an instruction to cancel,
// producing 266 words about an empty frame. Their own README says why — the
// format holds because that model is FINE-TUNED for it, with thinking
// enabled; a stock llm handed the same layout writes correct structure and
// dead pictures. It also costs 10-14 s instead of 3.
//
// What we did take from their spec: no quality boosters (with guidance off
// on this model such words push nothing and only dilute the description —
// see the image model registry in ai-api-stuff), present tense, and describe
// rather than instruct. Their rule on quoting text in its own script we had
// already arrived at on our own.
//
// Not taken, and worth its own task: their rewriter also PICKS AN ASPECT
// RATIO (3:2, 2:3, 1:1, 9:16). Our frame is nailed to 1152x896 in the
// service registry and the bot does not send a size for pictures at all, so
// a portrait subject wastes half the canvas.
const enhanceSystemPrompt = "You rewrite a user's short idea into ONE vivid English prompt for a text-to-image model.\n" +
	"RULES, in order of importance:\n" +
	"1. Keep the subject and every named thing exactly as given. Never replace, generalise or add a different main subject. Proper nouns you do NOT recognise — shops, brands, towns, people, invented or slang words — are kept as they are, transliterated into Latin letters if needed. Never swap an unfamiliar name for a familiar one: if you do not know what «Хофф» is, write Hoff, never IKEA. If a word looks invented, keep it invented.\n" +
	"2. If the idea is physically impossible, KEEP IT IMPOSSIBLE. Make it look like a real photograph of the impossible thing. Never relocate it to a plausible setting, never explain it away, never add words that undo it.\n" +
	"3. Keep the intent. A joke stays a joke, a meme stays a meme: make it look good, do not make it solemn or cinematic.\n" +
	"4. Text in quotes is copied character for character, in its original language, with the SAME quote marks: «» stay «», \"\" stay \"\". Never translate quoted text, never restyle its punctuation.\n" +
	"5. If the idea is a slogan, a caption, a headline or a sign, the words themselves must APPEAR in the picture: say where they are written — a poster, a banner, a shop sign — and quote them exactly, in their original language.\n" +
	"6. Add only: camera angle, lens feel, lighting, time of day, mood, materials, and one unexpected detail that makes the frame memorable. Never name a living artist, a studio or a franchise — describe the look in plain words instead. Never pad with quality words: no 'masterpiece', '8k', 'ultra detailed', 'award-winning', 'trending'.\n" +
	"7. If the idea is ALREADY detailed (names a style, a camera, lighting or runs longer than about 25 words), return it unchanged, translated to English if needed, keeping rule 4.\n" +
	"Write in the present tense, describing the finished picture as an observer sees it. Never address the reader and never give instructions.\n" +
	"Answer with the prompt only: 40-60 words, ONE flowing paragraph of full sentences, not a list of comma-separated tags. English, no preamble, no quotes around the whole answer."

// enhanceWordLimit is where a request stops being telegraphic. Taken from the
// journal rather than by eye: for pictures the 75th percentile is 7 words and
// the 90th is 23, so the gap between "wrote three words" and "wrote it out"
// sits right here. Longer requests go straight to the translator — rule 7
// tells the model to leave them alone anyway, and skipping the call saves the
// two and a half seconds.
const enhanceWordLimit = 25

// Sanity bounds for what comes back. The lower one catches a model that
// answered with a word or refused; the upper one catches a wall of text, which
// the encoder reads worse than a paragraph.
const (
	enhanceMinWords = 12
	enhanceMaxWords = 90
)

// How the prompt was built. Travels to the service in the "style" field and
// lands in the statistics, exactly where the style name used to: the column
// stays, so the new behaviour can be measured the same way the old one was.
const (
	promptModeEnhanced = "enhanced" // expanded by the model
	promptModeAsIs     = "asis"     // long enough already, only translated
	promptModeFallback = "fallback" // expansion was rejected, translation used
)

// quotedRe finds text the person put in quotes. Both kinds are matched: the
// bot is Russian and people type «ёлочки», but a phone keyboard gives "straight
// quotes" just as often.
var quotedRe = regexp.MustCompile(`«([^«»]+)»|"([^"]+)"`)

// restoreQuoted checks the quoted text survived the rewrite and puts the
// original quote marks back.
//
// This is done in code and not by asking nicer, because asking does not work:
// the instruction says in so many words to keep «» as «», and the model still
// normalises them to straight quotes about half the time. It matters because
// the picture model draws the quote marks it is given — a Russian greeting
// card then comes out with the wrong typography.
//
// Returns false when a quoted fragment vanished from the rewrite altogether.
// That is not a cosmetic problem: the person asked for those words to be in
// the picture, and a prompt that lost them is worse than no enhancement.
func restoreQuoted(enhanced, source string) (string, bool) {
	for _, match := range quotedRe.FindAllStringSubmatch(source, -1) {
		inner, open, closing := match[1], "«", "»"
		if inner == "" {
			inner, open, closing = match[2], `"`, `"`
		}

		if strings.TrimSpace(inner) == "" {
			continue
		}

		if !strings.Contains(enhanced, inner) {
			return enhanced, false
		}

		want := open + inner + closing
		if strings.Contains(enhanced, want) {
			continue
		}

		for _, wrong := range []string{`"` + inner + `"`, "“" + inner + "”", "'" + inner + "'"} {
			if strings.Contains(enhanced, wrong) {
				enhanced = strings.Replace(enhanced, wrong, want, 1)
				break
			}
		}
	}

	return enhanced, true
}

// acceptEnhanced decides whether the rewrite can be used.
//
// Every check here answers a failure seen on the live corpus, not a guess.
// Nothing is repaired beyond the quote marks: a rewrite that fails is dropped
// whole and the plain translation goes out instead, because half-repaired
// machine text is harder to debug than either end of the choice.
func acceptEnhanced(enhanced, source string) (string, bool) {
	enhanced = strings.Join(strings.Fields(enhanced), " ")

	if enhanced == "" {
		return "", false
	}

	if words := len(strings.Fields(enhanced)); words < enhanceMinWords || words > enhanceMaxWords {
		return "", false
	}

	return restoreQuoted(enhanced, source)
}

// buildImagePrompt turns what the person typed into the prompt the service
// gets, and names how it was built for the statistics.
//
// The plain translation is the floor, not an error path: whatever happens to
// the expansion — the llm is down, the answer is malformed, the quoted text
// got lost — the person still gets their picture, drawn from their own words.
func buildImagePrompt(source string) (string, string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", promptModeAsIs, nil
	}

	if len(strings.Fields(source)) > enhanceWordLimit {
		translated, err := translatePrompt(source)

		return translated, promptModeAsIs, err
	}

	enhanced, err := askLLM(enhanceSystemPrompt, source)
	if err == nil {
		if accepted, ok := acceptEnhanced(enhanced, source); ok {
			return accepted, promptModeEnhanced, nil
		}

		log.Printf("prompt enhancement rejected, falling back to translation: %q\n", enhanced)
	} else {
		log.Printf("prompt enhancement failed, falling back to translation: %v\n", err)
	}

	translated, terr := translatePrompt(source)

	return translated, promptModeFallback, terr
}
