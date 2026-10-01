package aiApi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// stickerBodyMaxTokens: ответ в одно слово, больше не нужно. Рассуждения
// выключены, поэтому маленький потолок безопасен — пустой ответ при малом
// потолке случается именно с включёнными рассуждениями.
const stickerBodyMaxTokens = 8

// stickerBodyPrompt asks the one question whose answer decides whether the
// pack may move the person's body.
//
// Written in English and answered in English on purpose: the question is ours,
// not the user's, and this model reads Latin more reliably than Cyrillic.
const stickerBodyPrompt = "Look at the photo and answer with ONE word and nothing else. " +
	"Answer BODY if the person's shoulders, chest or arms are visible. " +
	"Answer HEAD if only their head, face, neck or hair are visible."

// photoShowsBody says whether the photo shows more of the person than the head.
//
// Why it is asked at all: a sticker pack looks lifeless when every picture
// repeats the body of the source pixel for pixel, and naming a concrete pose
// for each emotion fixes that — but only when there IS a body in the frame. On
// a head-and-shoulders portrait the same instruction makes the model zoom out
// and invent a torso with clothes of its own; measured on four emotions
// 02.10.2026, all four grew a body.
//
// Asked ONCE per pack, not per sticker: the photo is the same for all ten.
//
// Any failure answers "no body", and that is the cautious side: the pack then
// behaves exactly as it did before this was added. A pack must not depend on
// the llm host being up.
func photoShowsBody(image []byte) bool {
	body, err := buildStickerBodyRequest(image)
	if err != nil {
		log.Println("[error] sticker pack: cannot build the body question", err)
		return false
	}

	answer, err := postLLM(body)
	if err != nil {
		log.Println("[error] sticker pack: the body question went unanswered", err)
		return false
	}

	// Ищем слово, а не сравниваем целиком: модель нет-нет да и добавит точку
	// или кавычки, и строгое сравнение отправило бы нас в осторожную ветку на
	// ровном месте.
	return strings.Contains(strings.ToUpper(answer), "BODY")
}

func buildStickerBodyRequest(image []byte) ([]byte, error) {
	dataURL := fmt.Sprintf("data:%s;base64,%s",
		http.DetectContentType(image), base64.StdEncoding.EncodeToString(image))

	return json.Marshal(visionRequest{
		Messages: []visionMessage{
			{Role: "user", Content: []visionPart{
				{Type: "text", Text: stickerBodyPrompt},
				{Type: "image_url", ImageURL: &visionImageURL{URL: dataURL}},
			}},
		},
		ReasoningEffort: "none",
		MaxTokens:       stickerBodyMaxTokens,
	})
}
