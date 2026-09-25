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

// Style templates.
//
// The previous ones were Fooocus presets written for SDXL with CFG 5-7, where
// padding with "quality" tokens (masterpiece, 8k, highly detailed) worked as a
// lever: it pushed the conditional prediction away from the unconditional one.
// Now images are drawn by Z-Image Turbo with guidance_scale=0.0: there is no
// unconditional branch at all, nothing to push apart, and such words turn into
// ordinary description tokens that dilute what the person actually asked for.
//
// On top of that, Z-Image has an LLM-based text encoder: it reads a coherent
// sentence noticeably better than a comma-separated list. Hence templates
// written as sentences.
// imageStyle is a named look: the suffixes that turn it on and the template
// that applies it. Name and template live together on purpose -- the name
// travels to the service with every request and is what the statistics group
// by, so a table that cannot drift is worth more than two parallel lists.
type imageStyle struct {
	name     string
	suffixes []string
	template string
}

var imageStyles = []imageStyle{
	{
		"anime",
		[]string{animeSuffix, animeSuffixRu},
		"%s. Japanese anime key visual, clean cel shading, vivid saturated colours, expressive linework.",
	},
	{
		"realistic",
		[]string{realisticSuffix, realisticSuffixRu},
		"%s. Shot on 35mm film in natural daylight, shallow depth of field, realistic skin texture with visible pores, fine grain.",
	},
	{
		// The anchor on games carries most of the load here. Without it, with
		// only the lighting description, Z-Image draws an ordinary night street
		// with signs lit from within rather than neon. In the old fooocus
		// template the same role was played by the "reminiscent of cyberpunk
		// genre video games" tail.
		"cyberpunk",
		[]string{cyberpunkSuffix, cyberpunkSuffixRu},
		"%s. In the style of the Cyberpunk 2077 video game, Night City after dark: " +
			"glowing neon tube signs and holographic billboards in magenta, cyan and electric blue, " +
			"dense stacked signage crowding the street, volumetric haze, rain-slick asphalt " +
			"mirroring the glow, strong bloom and anamorphic lens flare, saturated high-contrast night.",
	},
	{
		"meha",
		[]string{mehaSuffix, mehaSuffixRu},
		"%s. Hard-surface mecha design, panelled armour plating, exposed hydraulics and cabling, brushed metal worn at the edges.",
	},
}

// defaultImageStyleName marks a request that asked for no style at all: the
// prompt goes to the model as the person wrote it.
const defaultImageStyleName = "plain"

const defaultImageTemplate = "%s"

// matchImageStyle finds the style the trailing keyword asks for, nil when none.
func matchImageStyle(msgText string) *imageStyle {
	text := strings.ToLower(msgText)

	for i := range imageStyles {
		for _, suffix := range imageStyles[i].suffixes {
			if strings.HasSuffix(text, suffix) {
				return &imageStyles[i]
			}
		}
	}

	return nil
}

func getImageTemplate(msgText string) string {
	if style := matchImageStyle(msgText); style != nil {
		return style.template
	}

	return defaultImageTemplate
}

// getImageStyle names the applied template. Only statistics need this: it is
// how we learn which styles people reach for and which are dead weight.
func getImageStyle(msgText string) string {
	if style := matchImageStyle(msgText); style != nil {
		return style.name
	}

	return defaultImageStyleName
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

	return requestImage(enhancedPrompt, caller,
		promptOrigin{Source: msgText, Style: getImageStyle(msgText)})
}

// requestImage sends an already assembled prompt and waits for the finished
// image.
func requestImage(prompt string, caller Caller, origin promptOrigin) ([]byte, error) {
	escapedPrompt, err := json.Marshal(prompt)
	if err != nil {
		return nil, err
	}

	jsonStr := fmt.Appendf(nil, `{"prompt": %s%s%s}`, string(escapedPrompt), caller.userJSON(), origin.statsJSON())

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
