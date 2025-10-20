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
	log.Printf("got prompt %s\n", prompt)
	promptTemplate := getImageTemplate(msgText)
	translatedPrompt, err := translatePrompt(prompt)
	if err != nil {
		return nil, err
	}

	log.Printf("translated prompt %s\n", translatedPrompt)

	enhancedPrompt := fmt.Sprintf(promptTemplate, translatedPrompt)

	escapedPrompt, err := json.Marshal(enhancedPrompt)
	if err != nil {
		return nil, err
	}
	enhancedPrompt = string(escapedPrompt)

	var jsonStr = []byte(`{"sd_model_checkpoint": "flux1DevNSFWUNLOCKEDFp8_v20FP8.safetensors","CLIP_stop_at_last_layers": 2}`)

	url := fmt.Sprintf("%s/sdapi/v1/options", os.Getenv("AI_PAINTER_HOST"))
	_, err = http.Post(url, "application/json", bytes.NewReader(jsonStr))
	if err != nil {
		return nil, err
	}

	str := fmt.Sprintf(`{"prompt": %s,"batch_size": 1,"steps": 20,"seed": -1,"distilled_cfg_scale": 3.5,"cfg_scale": 1,"width": 1152,"height": 896,"sampler_name": "Euler","scheduler": "Simple"}`, enhancedPrompt)
	jsonStr = []byte(str)

	url = fmt.Sprintf("%s/sdapi/v1/txt2img", os.Getenv("AI_PAINTER_HOST"))
	res, err := http.Post(url, "application/json", bytes.NewReader(jsonStr))
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var jsonRes map[string]any               // declaring a map for key names as string and values as interface
	err = json.Unmarshal(resBytes, &jsonRes) // Unmarshalling
	if err != nil {
		return nil, err
	}

	imgSlice, ok := jsonRes["images"].([]interface{})

	if !ok {
		return nil, errors.New("wrong responce format")
	}

	if len(imgSlice) == 0 {
		return nil, errors.New("wrong responce format")
	}

	base64img := imgSlice[0].(string)

	decodedBytes, err := base64.StdEncoding.DecodeString(base64img)
	if err != nil {
		return nil, err
	}

	return decodedBytes, nil

}
