package aiApi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

const (
	imagePollInterval = 5 * time.Second
	imageMaxWait      = time.Hour
)

var animeSuffix = " anime"
var animeSuffixRu = " аниме"
var realisticSuffix = " realistic"
var realisticSuffixRu = " реалистично"
var cyberpunkSuffix = " cyberpunk"
var cyberpunkSuffixRu = " киберпанк"
var mehaSuffix = " meha"
var mehaSuffixRu = " меха"

// Шаблоны стилей.
//
// Прежние были пресетами Fooocus и писались под SDXL с CFG 5-7, где набивка
// «качественными» токенами (masterpiece, 8k, highly detailed) работала как
// рычаг: она растаскивала условный прогноз с безусловным. Сейчас картинки
// рисует Z-Image Turbo с guidance_scale=0.0 — безусловной ветки нет вовсе,
// растаскивать нечего, и такие слова превращаются в обычные токены описания,
// которые разбавляют собой то, что человек реально попросил.
//
// Плюс у Z-Image текстовый энкодер на базе LLM: связную фразу он читает
// заметно лучше, чем список через запятую. Отсюда шаблоны предложениями.
func getImageTemplate(msgText string) string {
	animeMeassageTemplate := "%s. Japanese anime key visual, clean cel shading, vivid saturated colours, expressive linework."
	realisticMessageTemplate := "%s. Shot on 35mm film in natural daylight, shallow depth of field, realistic skin texture with visible pores, fine grain."
	// Якорь на игру здесь несёт основную нагрузку. Без него — с одним лишь
	// описанием освещения — Z-Image рисует обычную ночную улицу с вывесками,
	// подсвеченными изнутри, а не неон. В прежнем fooocus-шаблоне ту же роль
	// играл хвост «reminiscent of cyberpunk genre video games».
	cyberpunkMessageTemplate := "%s. In the style of the Cyberpunk 2077 video game, Night City after dark: " +
		"glowing neon tube signs and holographic billboards in magenta, cyan and electric blue, " +
		"dense stacked signage crowding the street, volumetric haze, rain-slick asphalt " +
		"mirroring the glow, strong bloom and anamorphic lens flare, saturated high-contrast night."
	initialMessageTemplate := "%s"
	mehaMessageTemplate := "%s. Hard-surface mecha design, panelled armour plating, exposed hydraulics and cabling, brushed metal worn at the edges."

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

func generateImage(msgText string, caller Caller) ([]byte, error) {
	prompt := getImagePrompt(msgText)
	log.Printf("Got prompt %s\n", prompt)
	promptTemplate := getImageTemplate(msgText)
	translatedPrompt, err := translatePrompt(prompt)
	if err != nil {
		return nil, err
	}

	enhancedPrompt := applyPromptTemplate(promptTemplate, translatedPrompt)
	log.Printf("Image prompt: %s\n", enhancedPrompt)

	return requestImage(enhancedPrompt, caller)
}

// requestImage отправляет уже собранный промпт и ждёт готовую картинку.
func requestImage(prompt string, caller Caller) ([]byte, error) {
	escapedPrompt, err := json.Marshal(prompt)
	if err != nil {
		return nil, err
	}

	jsonStr := fmt.Appendf(nil, `{"prompt": %s%s}`, string(escapedPrompt), caller.userJSON())

	url := fmt.Sprintf("%s/api/txt2img", os.Getenv("AI_PAINTER_HOST"))
	res, err := submitClient.Post(url, "application/json", bytes.NewReader(jsonStr))
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if err := checkSubmitStatus("image", res.StatusCode, resBytes); err != nil {
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

	return waitResult("image", os.Getenv("AI_PAINTER_HOST"), id, imagePollInterval, imageMaxWait, caller.Progress)
}
