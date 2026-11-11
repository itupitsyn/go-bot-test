package aiApi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// Animating a photo used to be done blind.
//
// In the journal 101 video jobs of 164 are i2v — a person sends a picture and
// a caption. The picture goes to the generator, but the PLAN never saw it: it
// read the caption and wrote a scene for a subject it could only guess at. H3
// then had to reconcile our invented scene with a photo nobody had described.
//
// Our own llm has eyes (mmproj on the llm host), and H3's text encoder does
// not. So the one thing we can do that the generator cannot is look at the
// picture first and say what is on it.
//
// It also answers a question the text alone cannot. Four of the stable form
// misses measured on 11.11.2026 are bare phrases — "верим Альхамдулиллах",
// "Здравствуйте! Палилула Сырная маленькая к 17:00" — where nothing in the
// words says whether they are meant to be spoken. A photo of a person's face
// under such a caption says it plainly.

// photoDescriptionMaxTokens keeps the description to a couple of sentences.
// The plan reads it as context, and a long one would drown the request.
const photoDescriptionMaxTokens = 200

// photoDescriptionPrompt asks for the first frame, not for a review. What is
// wanted is the subject and the setting in plain words: the plan will build
// the motion, and the picture itself goes to the generator anyway.
const photoDescriptionPrompt = "You describe a photograph that is about to be " +
	"animated as the first frame of a short video.\n" +
	"Answer in ENGLISH, one or two sentences, 30 words at most.\n" +
	"Say who or what is in the frame, what they are doing, where it happens " +
	"and how it is framed (close-up, waist up, wide shot).\n" +
	"If a person is visible, say so plainly and whether the face is turned " +
	"to the camera.\n" +
	"Describe only what is in the picture. Do not guess at names, do not " +
	"invent a story, do not judge, do not mention the photograph itself."

func buildPhotoDescriptionRequest(image []byte) ([]byte, error) {
	dataURL := fmt.Sprintf("data:%s;base64,%s", http.DetectContentType(image),
		base64.StdEncoding.EncodeToString(image))

	return json.Marshal(visionRequest{
		Messages: []visionMessage{
			{Role: "system", Content: photoDescriptionPrompt},
			{Role: "user", Content: []visionPart{
				{Type: "text", Text: "What is in this picture?"},
				{Type: "image_url", ImageURL: &visionImageURL{URL: dataURL}},
			}},
		},
		ReasoningEffort: "none",
		MaxTokens:       photoDescriptionMaxTokens,
	})
}

// describeVideoPhoto says what is on the picture being animated.
//
// An error is never fatal here and is deliberately not returned: a blind plan
// is what we had until now, and it made videos. Losing somebody's video
// because the describing call failed would be a worse trade than describing
// nothing.
func describeVideoPhoto(image []byte) string {
	if len(image) == 0 {
		return ""
	}

	body, err := buildPhotoDescriptionRequest(image)
	if err != nil {
		log.Printf("video photo: cannot build the request (%v), planning blind\n", err)

		return ""
	}

	answer, err := postLLM(body)
	if err != nil {
		log.Printf("video photo: the llm did not describe it (%v), planning blind\n", err)

		return ""
	}

	answer = tidy(stripMarkers(answer))
	// An empty answer at a low token limit is a known habit of this model, and
	// an answer longer than the description it was asked for is of no use to
	// the plan either.
	if answer == "" || len(strings.Fields(answer)) > 60 {
		log.Printf("video photo: the description is unusable (%d words), planning blind\n",
			len(strings.Fields(answer)))

		return ""
	}

	return answer
}

// photoNote hands the description to the plan. It goes into the user half of
// the conversation, next to the request it belongs to, for the same reason the
// budget does: it changes with every request, and a system prompt that changes
// per request cannot be tested.
const photoNote = "\n\n(К просьбе приложена фотография — именно она станет " +
	"первым кадром. На ней: %s. Сцена должна быть про то, что на снимке, а не " +
	"про выдуманную обстановку. Если на фотографии человек, а в просьбе есть " +
	"фраза без пометок, это почти наверняка просьба произнести её вслух.)"

func photoContext(photo string) string {
	if photo == "" {
		return ""
	}

	return fmt.Sprintf(photoNote, strings.TrimRight(photo, " ."))
}
