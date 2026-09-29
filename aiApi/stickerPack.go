package aiApi

import (
	"errors"
	"fmt"
	"log"
	"os"
)

// StickerEmotion is one sticker of the standard pack: what to ask the model
// for and which emoji the result answers to in Telegram.
//
// The set is made of reactions people actually send to a chat, not of
// textbook emotions. "Sad" and "angry" look right in a psychology handbook;
// what gets sent is a facepalm and a thumbs up.
//
// The instructions are written in English and go to the service as they are,
// bypassing the translation that a hand-typed edit goes through: these are our
// own words, not the user's, and the edit model reads English best anyway.
//
// Every instruction ends with the same demand to keep the face, the hair and
// the clothes. Without it the model drifts a little on each one, and drift
// that is invisible in a single picture becomes obvious when ten of them sit
// side by side in the sticker panel.
type StickerEmotion struct {
	Key         string
	Emoji       []string
	Instruction string
}

// Nothing here names a particular feature on purpose. The first version said
// "keep the same face, hairstyle, beard and clothes" — written while looking
// at a bearded test subject — and the model did as told: it drew a beard on a
// woman who had none. An instruction to keep a feature reads as a promise that
// the feature is there. So the demand is about the person, not about parts.
const keepIdentity = ", keep the same person, do not change their face, hair " +
	"or clothes, keep the same camera framing and background"

var stickerEmotions = []StickerEmotion{
	{"laugh", []string{"😂"},
		"make this person laugh out loud, head tilted back, eyes squeezed shut" + keepIdentity},
	{"approve", []string{"👍"},
		"make this person smile confidently and give a thumbs up to the camera" + keepIdentity},
	{"deadpan", []string{"😐"},
		"make this person stare at the camera with a completely blank deadpan face" + keepIdentity},
	{"angry", []string{"😡"},
		"make this person furious, brows drawn together, jaw clenched, glaring" + keepIdentity},
	{"sad", []string{"😢"},
		"make this person look miserable, mouth turned down, watery eyes" + keepIdentity},
	{"squint", []string{"🤨"},
		"make this person squint at the camera with one raised eyebrow, sceptical" + keepIdentity},
	{"facepalm", []string{"🤦"},
		"make this person cover their face with one palm in despair" + keepIdentity},
	{"sleep", []string{"😴"},
		"make this person asleep, eyes closed, head drooping, mouth slightly open" + keepIdentity},
	{"think", []string{"🤔"},
		"make this person think hard, hand on chin, looking up and aside" + keepIdentity},
	{"delight", []string{"🤩"},
		"make this person beam with delight, wide open shining eyes, huge grin" + keepIdentity},
}

// StickerResult is one finished sticker.
type StickerResult struct {
	Emotion StickerEmotion
	PNG     []byte
}

// stickerMinReady is how many must come out for the pack to be worth making.
// Telegram is happy with one, but a pack of two is not a pack.
const stickerMinReady = 4

// StickerPackSize is how many pictures a pack costs. The bot shows it in the
// waiting message, so the number must come from here rather than be typed in
// again next to the text.
func StickerPackSize() int {
	return len(stickerEmotions)
}

// GenerateStickers turns one photo into the standard pack.
//
// The requests go ONE AT A TIME on purpose. The service refuses a person who
// already has max_user_inflight (five) jobs in flight, so a pack of ten sent
// as one burst would collect 429s from the sixth on. Sequential submission
// also gives the caller honest progress and leaves the queue free for
// everyone else between pictures: the scheduler hands each person two jobs in
// a row at most, so a pack does not freeze the chat.
//
// A single failed emotion is skipped rather than fatal: nine stickers are
// better than an error message. It gives up only when too few came out.
func GenerateStickers(image EditImage, caller Caller,
	onProgress func(done, total int)) ([]StickerResult, error) {
	if len(image.Bytes) == 0 {
		return nil, errors.New("no image for the sticker pack")
	}

	host := os.Getenv("AI_PAINTER_HOST")
	total := len(stickerEmotions)
	ready := make([]StickerResult, 0, total)

	for _, emotion := range stickerEmotions {
		// The origin keeps the pack visible in the statistics: source is the
		// emotion key, so it is clear later what people generate and which of
		// the ten fail more often than the rest.
		origin := promptOrigin{Source: "sticker:" + emotion.Key, Style: "sticker"}

		id, err := getEditId(emotion.Instruction, []EditImage{image}, caller, origin, true)
		if err != nil {
			// A refusal over the cap means somebody else's jobs are ours too —
			// there is no point in hammering the rest of the pack into it.
			if errors.Is(err, ErrQueueFull) {
				return ready, err
			}
			log.Printf("sticker %s not submitted: %v\n", emotion.Key, err)
			continue
		}

		png, err := waitResult("sticker", host, id, imagePollInterval, imageMaxWait, nil)
		if err != nil {
			log.Printf("sticker %s failed: %v\n", emotion.Key, err)
			continue
		}

		ready = append(ready, StickerResult{Emotion: emotion, PNG: png})
		if onProgress != nil {
			onProgress(len(ready), total)
		}
	}

	if len(ready) < stickerMinReady {
		return ready, fmt.Errorf("only %d of %d stickers came out", len(ready), total)
	}

	return ready, nil
}
