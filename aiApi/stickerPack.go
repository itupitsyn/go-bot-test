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
	// Pose is the body language for this emotion, added only when the photo
	// shows a body. Every emotion has one. The first version left it empty for
	// the three that already name a hand, on the reasoning that a hand moves
	// the body by itself; that reasoning was never measured and was wrong —
	// see the comment above approve.
	Pose string
}

// Prompt assembles what actually goes to the service.
//
// withBody decides both halves. With a body in frame the pose is named and the
// identity clause allows movement; without one, nothing is said about the body
// and the clause forbids changing the clothes, which pins the frame.
func (e StickerEmotion) Prompt(withBody bool) string {
	if !withBody {
		return e.Instruction + greenScreen + keepIdentity
	}

	instruction := e.Instruction
	if e.Pose != "" {
		instruction += ", " + e.Pose
	}

	return instruction + greenScreen + keepIdentityPosed
}

// greenScreen просит заменить фон на хромакей, и это НЕ прихоть кадра: по нему
// на стороне сервиса вырезается фон (см. cutout.py, _GREEN_*).
//
// Зачем так. Сегментирующая сеть не достаёт между прядями волос — просветы она
// заливает человеком, и в стикере между волосами остаются куски исходного фона.
// Модель правки, в отличие от неё, просовывает зелёный именно туда: она не
// ищет границу, она рисует картинку заново. Вырезать ровный цвет потом —
// арифметика, а не угадывание.
//
// Сервис к этой строке не привязан жёстко: зелёного в кадре нет — он режет
// по-старому, маской. Поэтому бот и сервис можно обновлять порознь.
const greenScreen = ", replace the background behind them with a flat solid " +
	"chroma key green screen"

// Nothing here names a particular feature on purpose. The first version said
// "keep the same face, hairstyle, beard and clothes" — written while looking
// at a bearded test subject — and the model did as told: it drew a beard on a
// woman who had none. An instruction to keep a feature reads as a promise that
// the feature is there. So the demand is about the person, not about parts.
// Nothing is said about the pose or the framing, and that is deliberate too.
// The first version demanded "keep the same camera framing and background",
// added back when a drifting crop was a real problem. It stopped being one:
// cutout.py squares every sticker around the subject, so the framing is
// normalised after the fact. What the demand did keep doing was fighting the
// emotions — a facepalm needs a raised hand, sleeping needs a drooping head,
// and an order to hold the camera still made them stiff.
// Skin tone and lighting are named, and naming them is safe where naming a
// beard was not: everyone has both, so asking to keep them invents nothing.
// The reason they are here: on a photo with dark hair the model painted the
// sleeper's own arm dark blue-grey, blending it into the hair behind — the arm
// belonged in the frame, only its colour did not (seen live 29.09.2026).
const keepIdentity = ", keep the same person, do not change their face, hair, " +
	"skin tone or clothes, keep the same lighting"

// keepIdentityPosed is the same demand for a photo that shows a body, with one
// word changed: "do not change their clothes" becomes "wearing the same
// clothes".
//
// That one word was what froze the pack. Clothes are the torso, and forbidding
// any change to them forbids the body to move at all: measured 02.10.2026, the
// lower half of the frame differed from the source by 3.1 of 255 across four
// emotions — that is "did not stir". With this wording and a named pose it is
// 43.5, and the difference is a live pose rather than a redrawn frame.
//
// What does NOT work, so that nobody tries it again: an abstract permission
// ("let their posture shift naturally with the feeling") does not move the
// person. The model reframes instead — the figure comes out smaller and lower,
// which inflates any measurement while the pose stays the same. Only a
// concrete, named pose moves the body.
const keepIdentityPosed = ", keep the same person: same face, same hair, " +
	"same skin tone, wearing the same clothes, same lighting"

var stickerEmotions = []StickerEmotion{
	{Key: "laugh", Emoji: []string{"😂"},
		Instruction: "make this person laugh out loud, head tilted back, eyes squeezed shut",
		Pose:        "shoulders thrown back"},
	// A named hand does NOT move the body. It was tempting to think it does —
	// a thumbs up, a palm over the face and a hand on the chin all sound like
	// whole-body gestures — so the first version gave these three no pose and
	// shipped. On the live pack the torso came out pixel-identical to the
	// source on every one of them. Measured against the photo (lower 45% of
	// the frame, 0–255): thumbs up 18.7, facepalm 18.5, hand on chin 35.0.
	// With a pose named: 55.1, 57.0, 54.9. The model moves the arm and leaves
	// the body exactly where it found it unless the body is addressed too.
	{Key: "approve", Emoji: []string{"👍"},
		Instruction: "make this person smile confidently and give a thumbs up to the camera",
		Pose:        "leaning in towards the camera, one shoulder turned forward"},
	{Key: "deadpan", Emoji: []string{"😐"},
		Instruction: "make this person stare at the camera with a completely blank deadpan face",
		Pose:        "shoulders square to the camera, perfectly still"},
	{Key: "angry", Emoji: []string{"😡"},
		Instruction: "make this person furious, brows drawn together, jaw clenched, glaring",
		Pose:        "shoulders squared and leaning in towards the camera"},
	{Key: "sad", Emoji: []string{"😢"},
		Instruction: "make this person look miserable, mouth turned down, watery eyes",
		Pose:        "shoulders slumped and turned away a little"},
	{Key: "squint", Emoji: []string{"🤨"},
		Instruction: "make this person squint at the camera with one raised eyebrow, sceptical",
		Pose:        "upper body turned to one side, leaning back away from the camera"},
	{Key: "facepalm", Emoji: []string{"🤦"},
		Instruction: "make this person cover their face with one palm in despair",
		Pose:        "head bowed into the palm, shoulders drawn up and turned away a little"},
	// "head drooping" is gone on purpose. A drooping head needs something to
	// droop onto, and the model obliges: on a photo with long dark hair it
	// grew a whole extra arm along the body, dark enough to be taken for the
	// person's own skin (seen twice on live packs, 29.09.2026). Sleeping
	// peacefully asks for the same thing without inviting a support.
	//
	// Поза по той же причине описывает только голову и плечи: всё, что просит
	// опереться или уронить, зовёт лишнюю руку. Первый вариант, "shoulders
	// relaxed", был ещё осторожнее — и не двигал вообще ничего (2.8 из 255,
	// это замороженный кадр). Наклон головы даёт 36.2 и руки не растит.
	{Key: "sleep", Emoji: []string{"😴"},
		Instruction: "make this person sleep peacefully, eyes closed, relaxed face, " +
			"mouth slightly open",
		Pose: "head tilted to one side, shoulders dropped"},
	{Key: "think", Emoji: []string{"🤔"},
		Instruction: "make this person think hard, hand on chin, looking up and aside",
		Pose:        "upper body turned a little away from the camera"},
	{Key: "delight", Emoji: []string{"🤩"},
		Instruction: "make this person beam with delight, wide open shining eyes, huge grin",
		Pose: "leaning back with shoulders raised and both hands clasped " +
			"near the chest"},
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

	// Спрашиваем ОДИН раз на набор: фотография у всех десяти одна и та же.
	// Ответ решает, можно ли называть позу — см. photoShowsBody.
	withBody := photoShowsBody(image.Bytes)
	log.Printf("sticker pack: body in frame = %v\n", withBody)

	for _, emotion := range stickerEmotions {
		// The origin keeps the pack visible in the statistics: source is the
		// emotion key, so it is clear later what people generate and which of
		// the ten fail more often than the rest.
		origin := promptOrigin{Source: "sticker:" + emotion.Key, Style: "sticker"}

		id, err := getEditId(emotion.Prompt(withBody), []EditImage{image}, caller, origin, true)
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
