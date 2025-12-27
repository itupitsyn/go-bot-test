package aiApi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

var animeSuffix = " anime"
var animeSuffixRu = " аниме"
var realisticSuffix = " realistic"
var realisticSuffixRu = " реалистично"
var cyberpunkSuffix = " cyberpunk"
var cyberpunkSuffixRu = " киберпанк"
var mehaSuffix = " meha"
var mehaSuffixRu = " меха"

func getImageTemplate(msgText string) string {
	animeMeassageTemplate := "%s . anime style, key visual, vibrant, studio anime, highly detailed"
	realisticMessageTemplate := "%s, RAW candid cinema, 16mm, color graded portra 400 film, remarkable color, ultra realistic, textured skin, remarkable detailed pupils, realistic dull skin noise, visible skin detail, skin fuzz, dry skin, shot with cinematic camera"
	cyberpunkMessageTemplate := "%s . neon, dystopian, futuristic, digital, vibrant, detailed, high contrast, reminiscent of cyberpunk genre video games"
	initialMessageTemplate := "%s"
	mehaMessageTemplate := "%s . it should look like it does in a real ife . blend of organic and mechanical elements, futuristic, cybernetic, detailed, intricate"

	text := strings.ToLower(msgText)

	var messageTemplate string
	if strings.HasSuffix(text, animeSuffix) || strings.HasSuffix(text, animeSuffixRu) {
		messageTemplate = animeMeassageTemplate
	} else if strings.HasSuffix(text, realisticSuffix) || strings.HasSuffix(text, realisticSuffixRu) {
		messageTemplate = realisticMessageTemplate
	} else if strings.HasSuffix(text, cyberpunkSuffix) || strings.HasSuffix(text, cyberpunkSuffixRu) {
		messageTemplate = cyberpunkMessageTemplate
	} else if strings.HasSuffix(text, mehaSuffix) || strings.HasSuffix(text, mehaSuffixRu) {
		messageTemplate = mehaMessageTemplate
	} else {
		messageTemplate = initialMessageTemplate
	}

	return messageTemplate
}

func getImagePrompt(msgText string) string {
	text := strings.ToLower(msgText)

	if strings.HasSuffix(text, animeSuffix) || strings.HasSuffix(text, animeSuffixRu) {
		text, _ = strings.CutSuffix(text, animeSuffix)
		text, _ = strings.CutSuffix(text, animeSuffixRu)
	} else if strings.HasSuffix(text, realisticSuffix) || strings.HasSuffix(text, realisticSuffixRu) {
		text, _ = strings.CutSuffix(text, realisticSuffix)
		text, _ = strings.CutSuffix(text, realisticSuffixRu)
	} else if strings.HasSuffix(text, cyberpunkSuffix) || strings.HasSuffix(text, cyberpunkSuffixRu) {
		text, _ = strings.CutSuffix(text, cyberpunkSuffix)
		text, _ = strings.CutSuffix(text, cyberpunkSuffixRu)
	} else if strings.HasSuffix(text, mehaSuffix) || strings.HasSuffix(text, mehaSuffixRu) {
		text, _ = strings.CutSuffix(text, mehaSuffix)
		text, _ = strings.CutSuffix(text, mehaSuffixRu)
	}

	text, _ = strings.CutPrefix(text, "draw ")
	text, _ = strings.CutPrefix(text, "нарисуй ")

	return text
}

func generateImage(msgText string) ([]byte, error) {
	prompt := getImagePrompt(msgText)
	log.Printf("Got prompt %s\n", prompt)
	promptTemplate := getImageTemplate(msgText)
	translatedPrompt, err := translatePrompt(prompt)
	if err != nil {
		return nil, err
	}

	enhancedPrompt := fmt.Sprintf(promptTemplate, translatedPrompt)
	escapedPrompt, err := json.Marshal(enhancedPrompt)

	log.Printf("Translated prompt %s\n", escapedPrompt)
	if err != nil {
		return nil, err
	}
	enhancedPrompt = string(escapedPrompt)

	str := fmt.Sprintf(`{"prompt": %s}`, enhancedPrompt)
	jsonStr := []byte(str)

	url := fmt.Sprintf("%s/api/txt2img", os.Getenv("AI_PAINTER_HOST"))
	res, err := http.Post(url, "application/json", bytes.NewReader(jsonStr))
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var jsonRes map[string]any
	err = json.Unmarshal(resBytes, &jsonRes)
	if err != nil {
		return nil, err
	}

	id, ok := jsonRes["id"].(string)
	if !ok {
		return nil, errors.New("wrong response format while getting id")
	}

	log.Println("Start waiting for image generation result")
	i := 0
	for {
		if i == 200 {
			return nil, errors.New("Waiting for image generation is too long")
		}

		url := fmt.Sprintf("%s/api/result", os.Getenv("AI_PAINTER_HOST"))
		res, err := http.Get(fmt.Sprintf("%s?id=%s", url, id))
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()

		resBytes, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(resBytes, &jsonRes) // Unmarshalling
		if err != nil {
			return nil, err
		}

		status, ok := jsonRes["status"].(string)
		if !ok {
			return nil, errors.New("wrong response format while getting generation status")
		}

		if status == "pending" || status == "in_progress" {
			time.Sleep(5 * time.Second)
			continue
		} else if status == "error" {
			return nil, errors.New("error during image generation")
		}

		break
	}

	base64img, ok := jsonRes["data"].(string)
	if !ok {
		return nil, errors.New("wrong response format while getting image data")
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(base64img)
	if err != nil {
		return nil, err
	}

	return decodedBytes, nil
}
