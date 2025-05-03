package aiApi

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"math/rand"

	"github.com/gorilla/websocket"
)

type msgResponse struct {
	Msg string `json:"msg"`
}

type msgImageGeneratedRespons struct {
	Msg    string `json:"msg"`
	Output struct {
		Data []struct {
			Value []struct {
				Name string `json:"name,omitempty"`
			} `json:"value"`
		} `json:"data"`
	} `json:"output"`
}

var animeSuffix = " anime"
var animeSuffixRu = " аниме"
var realisticSuffix = " realistic"
var realisticSuffixRu = " реалистично"
var cyberpunkSuffix = " cyberpunk"
var cyberpunkSuffixRu = " киберпанк"
var mehaSuffix = " meha"
var mehaSuffixRu = " меха"

func getWSConnection() (*websocket.Conn, error) {
	u := url.URL{Scheme: "wss", Host: os.Getenv("AI_PAINTER_HOST"), Path: "/queue/join"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func getImageTemplate(msgText string) string {
	animeMeassageTemplate := "{\"data\":[null,false,\"%s\",\"\",[\"MRE Anime\"],\"Speed\",\"1152×896 <span style=\\\"color: grey;\\\"> ∣ 9:7</span>\",1,\"png\",\"%s\",false,2,4,\"juggernautXL_v8Rundiffusion.safetensors\",\"None\",0.5,true,\"sd_xl_offset_example-lora_1.0.safetensors\",0.1,true,\"None\",1,true,\"None\",1,true,\"None\",1,true,\"None\",1,false,\"uov\",\"Disabled\",null,[],null,\"\",null,false,false,false,false,1.5,0.8,0.3,7,2,\"dpmpp_2m_sde_gpu\",\"karras\",\"Default (model)\",-1,-1,-1,-1,-1,-1,false,false,false,false,64,128,\"joint\",0.25,false,1.01,1.02,0.99,0.95,false,false,\"v2.6\",1,0.618,false,false,0,false,false,\"fooocus\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",false,0,false,null,false,\"Disabled\",\"Before First Enhancement\",\"Original Prompts\",false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false],\"event_data\":null,\"fn_index\":67,\"session_hash\":\"%s\"}"
	realisticMessageTemplate := "{\"data\":[null,false,\"%s\",\"unrealistic, saturated, high contrast, big nose, painting, drawing, sketch, cartoon, anime, manga, render, CG, 3d, watermark, signature, label\",[\"Fooocus V2\",\"Fooocus Photograph\",\"Fooocus Negative\"],\"Speed\",\"896×1152 <span style=\\\"color: grey;\\\"> ∣ 7:9</span>\",1,\"png\",\"%s\",false,2,3,\"realisticStockPhoto_v20.safetensors\",\"None\",0.5,true,\"SDXL_FILM_PHOTOGRAPHY_STYLE_V1.safetensors\",0.25,true,\"None\",1,true,\"None\",1,true,\"None\",1,true,\"None\",1,false,\"uov\",\"Disabled\",null,[],null,\"\",null,false,false,false,false,1.5,0.8,0.3,7,2,\"dpmpp_2m_sde_gpu\",\"karras\",\"Default (model)\",-1,-1,-1,-1,-1,-1,false,false,false,false,64,128,\"joint\",0.25,false,1.01,1.02,0.99,0.95,false,false,\"v2.6\",1,0.618,false,false,0,false,false,\"fooocus\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",false,0,false,null,false,\"Disabled\",\"Before First Enhancement\",\"Original Prompts\",false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false],\"event_data\":null,\"fn_index\":67,\"session_hash\":\"%s\"}"
	cyberpunkMessageTemplate := "{\"data\":[null,false,\"%s\",\"\",[\"Game Cyberpunk Game\"],\"Speed\",\"1152×896 <span style=\\\"color: grey;\\\"> ∣ 9:7</span>\",1,\"png\",\"%s\",false,2,4,\"juggernautXL_v8Rundiffusion.safetensors\",\"None\",0.5,true,\"sd_xl_offset_example-lora_1.0.safetensors\",0.1,true,\"None\",1,true,\"None\",1,true,\"None\",1,true,\"None\",1,false,\"uov\",\"Disabled\",null,[],null,\"\",null,false,false,false,false,1.5,0.8,0.3,7,2,\"dpmpp_2m_sde_gpu\",\"karras\",\"Default (model)\",-1,-1,-1,-1,-1,-1,false,false,false,false,64,128,\"joint\",0.25,false,1.01,1.02,0.99,0.95,false,false,\"v2.6\",1,0.618,false,false,0,false,false,\"fooocus\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",false,0,false,null,false,\"Disabled\",\"Before First Enhancement\",\"Original Prompts\",false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false],\"event_data\":null,\"fn_index\":67,\"session_hash\":\"%s\"}"
	initialMessageTemplate := "{\"data\":[null,false,\"%s\",\"\",[\"Fooocus V2\",\"Fooocus Enhance\",\"Fooocus Sharp\"],\"Speed\",\"1152×896 <span style=\\\"color: grey;\\\"> ∣ 9:7</span>\",1,\"png\",\"%s\",false,2,4,\"juggernautXL_v8Rundiffusion.safetensors\",\"None\",0.5,true,\"sd_xl_offset_example-lora_1.0.safetensors\",0.1,true,\"None\",1,true,\"None\",1,true,\"None\",1,true,\"None\",1,false,\"uov\",\"Disabled\",null,[],null,\"\",null,false,false,false,false,1.5,0.8,0.3,7,2,\"dpmpp_2m_sde_gpu\",\"karras\",\"Default (model)\",-1,-1,-1,-1,-1,-1,false,false,false,false,64,128,\"joint\",0.25,false,1.01,1.02,0.99,0.95,false,false,\"v2.6\",1,0.618,false,false,0,false,false,\"fooocus\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",false,0,false,null,false,\"Disabled\",\"Before First Enhancement\",\"Original Prompts\",false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false],\"event_data\":null,\"fn_index\":67,\"session_hash\":\"%s\"}"
	mehaMessageTemplate := "{\"data\":[null,false,\"%s\",\"\",[\"Futuristic Biomechanical Cyberpunk\"],\"Speed\",\"1152×896 <span style=\\\"color: grey;\\\"> ∣ 9:7</span>\",1,\"png\",\"%s\",false,2,4,\"juggernautXL_v8Rundiffusion.safetensors\",\"None\",0.5,true,\"sd_xl_offset_example-lora_1.0.safetensors\",0.1,true,\"None\",1,true,\"None\",1,true,\"None\",1,true,\"None\",1,false,\"uov\",\"Disabled\",null,[],null,\"\",null,false,false,false,false,1.5,0.8,0.3,7,2,\"dpmpp_2m_sde_gpu\",\"karras\",\"Default (model)\",-1,-1,-1,-1,-1,-1,false,false,false,false,64,128,\"joint\",0.25,false,1.01,1.02,0.99,0.95,false,false,\"v2.6\",1,0.618,false,false,0,false,false,\"fooocus\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",false,0,false,null,false,\"Disabled\",\"Before First Enhancement\",\"Original Prompts\",false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false],\"event_data\":null,\"fn_index\":67,\"session_hash\":\"%s\"}"

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

func initiateImageGeneration(hash string, messageText string) (bool, error) {
	c, err := getWSConnection()
	if err != nil {
		return false, err
	}

	data := &msgResponse{}
	_ = c.ReadJSON(data)

	defer c.Close()

	if data.Msg != "send_hash" {
		errorText := fmt.Sprintf("[error] error parsing data from server (%s)", "send_hash")
		return false, errors.New(errorText)
	}

	wsText := fmt.Sprintf("{\"fn_index\":67,\"session_hash\":\"%s\"}", hash)
	err = c.WriteMessage(websocket.TextMessage, []byte(wsText))
	if err != nil {
		return false, errors.Join(err, errors.New("[error] error sending hash"))
	}

	_ = c.ReadJSON(data) // estimation
	if data.Msg != "estimation" {
		return false, fmt.Errorf("[error] error parsing data from server (%s)", "estimation")
	}

	_ = c.ReadJSON(data) // send_data
	if data.Msg != "send_data" {
		return false, fmt.Errorf("[error] error parsing data from server (%s)", "send_data")
	}

	randomPart := ""
	for i := 0; i < 20; i++ {
		randomPart += fmt.Sprintf("%d", rand.Int31n(10))
	}

	prompt := getImagePrompt(messageText)
	messageTemplate := getImageTemplate(messageText)
	translatedText, err := translatePrompt(prompt)

	if err != nil {
		return false, err
	}

	messageText = fmt.Sprintf(messageTemplate, strings.ReplaceAll(translatedText, "\"", "\\\""), randomPart, hash)
	err = c.WriteMessage(websocket.TextMessage, []byte(messageText))
	defer c.Close()

	if err != nil {
		return false, errors.Join(err, errors.New("[error] error sending prompt"))
	}

	_ = c.ReadJSON(data) // process_starts
	if data.Msg != "process_starts" {
		errText := fmt.Sprintf("[error] error parsing data from server (%s)", "process_starts")
		return false, errors.New(errText)
	}

	_ = c.ReadJSON(data) // process_completed
	if data.Msg != "process_completed" {
		return false, fmt.Errorf("[error] error parsing data from server (%s)", "process_completed")
	}

	return true, nil
}

func processGenerationResult(hash string) (string, error) {
	c, err := getWSConnection()

	if err != nil {
		return "", errors.Join(err, errors.New("[error] error processing image generation"))
	}
	defer c.Close()

	data := &msgResponse{}
	_ = c.ReadJSON(data)
	if data.Msg != "send_hash" {
		return "", fmt.Errorf("[error] error parsing data from server (%s)", "send_hash")
	}

	messageText := fmt.Sprintf("{\"fn_index\":68,\"session_hash\":\"%s\"}", hash)
	err = c.WriteMessage(websocket.TextMessage, []byte(messageText))
	if err != nil {
		return "", errors.Join(err, errors.New("[error] error sending hash"))
	}

	_ = c.ReadJSON(data) // estimation
	if data.Msg != "estimation" {
		return "", fmt.Errorf("[error] error parsing data from server (%s)", "estimation")
	}

	_ = c.ReadJSON(data) // send_data
	if data.Msg != "send_data" {
		return "", fmt.Errorf("[error] error parsing data from server (%s)", "send_data")
	}

	messageText = fmt.Sprintf("{\"data\":[null],\"event_data\":null,\"fn_index\":68,\"session_hash\":\"%s\"}", hash)
	err = c.WriteMessage(websocket.TextMessage, []byte(messageText))
	if err != nil {
		return "", errors.New("[error] error sending data for getting result")
	}

	processData := &msgImageGeneratedRespons{}
	_ = c.ReadJSON(processData) // process_starts, process_generating, process_completed
	if processData.Msg != "process_starts" {
		return "", fmt.Errorf("[error] error parsing data from server (%s)", "process_starts")
	}

	for processData.Msg != "process_completed" {
		_ = c.ReadJSON(processData)
		if processData.Msg != "process_generating" && processData.Msg != "process_completed" {
			return "", fmt.Errorf("[error] error parsing data from server (%s)", "process_generating/process_completed")
		}
	}

	if len(processData.Output.Data) < 4 || len(processData.Output.Data[3].Value) < 1 {
		return "", errors.New("[error] error parsing generation result")
	}

	pathToImage := processData.Output.Data[3].Value[0].Name
	url := fmt.Sprintf("https://%s/file=%s", os.Getenv("AI_PAINTER_HOST"), pathToImage)

	return url, nil
}
