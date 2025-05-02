package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"strings"

	"net/http"
	"net/url"
	"os"

	"telebot/aiApi"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
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

func sendWaitMessage(ctx context.Context, b *bot.Bot, update *models.Update) error {
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		InlineMessageID: update.CallbackQuery.InlineMessageID,
		Text:            "ОЖИДАЕМ!!!",
	})
	return err
}

func processImgGenerationError(ctx context.Context, b *bot.Bot, update *models.Update) error {
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		InlineMessageID: update.CallbackQuery.InlineMessageID,
		Text:            "Нет, сервер подох!",
	})
	return err
}

func processCallbackQuery(ctx context.Context, b *bot.Bot, update *models.Update) {
	go func() {
		hash := string(uuid.NewString()[:8])
		success := initiateImageGeneration(ctx, b, update, hash)
		if !success {
			return
		}
		processGenerationResult(ctx, b, update, hash)
	}()
}

func processGenerationResult(ctx context.Context, b *bot.Bot, update *models.Update, hash string) {
	c, err := getWSConnection(ctx, b, update)

	if err != nil {
		log.Println("Error processing image generation", err)
		processImgGenerationError(ctx, b, update)
		return
	}
	defer c.Close()

	data := &msgResponse{}
	_ = c.ReadJSON(data)
	if data.Msg != "send_hash" {
		log.Printf("Error parsing data from server (%s)\n", "send_hash")
		processImgGenerationError(ctx, b, update)
		return
	}

	messageText := fmt.Sprintf("{\"fn_index\":68,\"session_hash\":\"%s\"}", hash)
	err = c.WriteMessage(websocket.TextMessage, []byte(messageText))
	if err != nil {
		log.Println("Error sending hash")
		processImgGenerationError(ctx, b, update)
		return
	}

	_ = c.ReadJSON(data) // estimation
	if data.Msg != "estimation" {
		log.Printf("Error parsing data from server (%s)\n", "estimation")
		processImgGenerationError(ctx, b, update)
		return
	}

	_ = c.ReadJSON(data) // send_data
	if data.Msg != "send_data" {
		log.Printf("Error parsing data from server (%s)\n", "send_data")
		processImgGenerationError(ctx, b, update)
		return
	}

	messageText = fmt.Sprintf("{\"data\":[null],\"event_data\":null,\"fn_index\":68,\"session_hash\":\"%s\"}", hash)
	err = c.WriteMessage(websocket.TextMessage, []byte(messageText))
	if err != nil {
		log.Println("Error sending data for getting result")
		processImgGenerationError(ctx, b, update)
		return
	}

	processData := &msgImageGeneratedRespons{}
	_ = c.ReadJSON(processData) // process_starts, process_generating, process_completed
	if processData.Msg != "process_starts" {
		log.Printf("Error parsing data from server (%s)\n", "process_starts")
		processImgGenerationError(ctx, b, update)
		return
	}

	for processData.Msg != "process_completed" {
		_ = c.ReadJSON(processData)
		if processData.Msg != "process_generating" && processData.Msg != "process_completed" {
			log.Printf("Error parsing data from server (%s)\n", "process_generating/process_completed")
			processImgGenerationError(ctx, b, update)
			return
		}
	}

	if len(processData.Output.Data) < 4 || len(processData.Output.Data[3].Value) < 1 {
		log.Println("Error parsing generation result")
		processImgGenerationError(ctx, b, update)
		return
	}

	pathToImage := processData.Output.Data[3].Value[0].Name
	url := fmt.Sprintf("https://%s/file=%s", os.Getenv("AI_PAINTER_HOST"), pathToImage)

	response, e := http.Get(url)
	if e != nil {
		defer response.Body.Close()
		log.Println("Error getting generated image")
		processImgGenerationError(ctx, b, update)
		return
	}

	defer response.Body.Close()

	imageBytes, e := io.ReadAll(response.Body)
	if e != nil {
		log.Println("Error reading generated image")
		processImgGenerationError(ctx, b, update)
		return
	}

	b.SendPhoto(ctx, &bot.SendPhotoParams{})

	res, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID: os.Getenv("TMP_CHAT_ID"),
		Photo:  &models.InputFileUpload{Filename: "photo", Data: bytes.NewReader(imageBytes)},
	})
	if err != nil {
		log.Println(err)
		processImgGenerationError(ctx, b, update)
		return
	}

	if len(res.Photo) == 0 {
		log.Println("Can't get uploaded image")
		processImgGenerationError(ctx, b, update)
		return
	}

	photo := &models.InputMediaPhoto{
		Media:      res.Photo[0].FileID,
		HasSpoiler: true,
		Caption:    update.CallbackQuery.Data,
	}

	_, err = b.EditMessageMedia(ctx, &bot.EditMessageMediaParams{
		InlineMessageID: update.CallbackQuery.InlineMessageID,
		Media:           photo,
	})

	if err != nil {
		var typeErr *json.UnmarshalTypeError
		if !errors.As(err, &typeErr) {
			log.Println("Error editing message")
			log.Println(err)
			processImgGenerationError(ctx, b, update)
		}
	}

	_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    res.Chat.ID,
		MessageID: res.ID,
	})
	if err != nil {
		log.Println("Error deleting message")
		log.Println(err)
		processImgGenerationError(ctx, b, update)
		return
	}
}

func getWSConnection(ctx context.Context, b *bot.Bot, update *models.Update) (*websocket.Conn, error) {
	u := url.URL{Scheme: "wss", Host: os.Getenv("AI_PAINTER_HOST"), Path: "/queue/join"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Println("dial:", err)
		processImgGenerationError(ctx, b, update)
		return nil, err
	}

	return c, nil
}

func initiateImageGeneration(ctx context.Context, b *bot.Bot, update *models.Update, hash string) bool {
	c, err := getWSConnection(ctx, b, update)
	if err != nil {
		return false
	}

	sendWaitMessage(ctx, b, update)

	data := &msgResponse{}
	_ = c.ReadJSON(data)

	defer c.Close()

	if data.Msg != "send_hash" {
		log.Printf("Error parsing data from server (%s)\n", "send_hash")
		processImgGenerationError(ctx, b, update)
		return false
	}

	messageText := fmt.Sprintf("{\"fn_index\":67,\"session_hash\":\"%s\"}", hash)
	err = c.WriteMessage(websocket.TextMessage, []byte(messageText))
	if err != nil {
		log.Println("Error sending hash")
		processImgGenerationError(ctx, b, update)
		return false
	}

	_ = c.ReadJSON(data) // estimation
	if data.Msg != "estimation" {
		log.Printf("Error parsing data from server (%s)\n", "estimation")
		processImgGenerationError(ctx, b, update)
		return false
	}

	_ = c.ReadJSON(data) // send_data
	if data.Msg != "send_data" {
		log.Printf("Error parsing data from server (%s)\n", "send_data")
		processImgGenerationError(ctx, b, update)
		return false
	}

	animeMeassageTemplate := "{\"data\":[null,false,\"%s\",\"\",[\"MRE Anime\"],\"Speed\",\"1152×896 <span style=\\\"color: grey;\\\"> ∣ 9:7</span>\",1,\"png\",\"%s\",false,2,4,\"juggernautXL_v8Rundiffusion.safetensors\",\"None\",0.5,true,\"sd_xl_offset_example-lora_1.0.safetensors\",0.1,true,\"None\",1,true,\"None\",1,true,\"None\",1,true,\"None\",1,false,\"uov\",\"Disabled\",null,[],null,\"\",null,false,false,false,false,1.5,0.8,0.3,7,2,\"dpmpp_2m_sde_gpu\",\"karras\",\"Default (model)\",-1,-1,-1,-1,-1,-1,false,false,false,false,64,128,\"joint\",0.25,false,1.01,1.02,0.99,0.95,false,false,\"v2.6\",1,0.618,false,false,0,false,false,\"fooocus\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",false,0,false,null,false,\"Disabled\",\"Before First Enhancement\",\"Original Prompts\",false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false],\"event_data\":null,\"fn_index\":67,\"session_hash\":\"%s\"}"
	realisticMessageTemplate := "{\"data\":[null,false,\"%s\",\"unrealistic, saturated, high contrast, big nose, painting, drawing, sketch, cartoon, anime, manga, render, CG, 3d, watermark, signature, label\",[\"Fooocus V2\",\"Fooocus Photograph\",\"Fooocus Negative\"],\"Speed\",\"896×1152 <span style=\\\"color: grey;\\\"> ∣ 7:9</span>\",1,\"png\",\"%s\",false,2,3,\"realisticStockPhoto_v20.safetensors\",\"None\",0.5,true,\"SDXL_FILM_PHOTOGRAPHY_STYLE_V1.safetensors\",0.25,true,\"None\",1,true,\"None\",1,true,\"None\",1,true,\"None\",1,false,\"uov\",\"Disabled\",null,[],null,\"\",null,false,false,false,false,1.5,0.8,0.3,7,2,\"dpmpp_2m_sde_gpu\",\"karras\",\"Default (model)\",-1,-1,-1,-1,-1,-1,false,false,false,false,64,128,\"joint\",0.25,false,1.01,1.02,0.99,0.95,false,false,\"v2.6\",1,0.618,false,false,0,false,false,\"fooocus\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",false,0,false,null,false,\"Disabled\",\"Before First Enhancement\",\"Original Prompts\",false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false],\"event_data\":null,\"fn_index\":67,\"session_hash\":\"%s\"}"
	cyberpunkMessageTemplate := "{\"data\":[null,false,\"%s\",\"\",[\"Game Cyberpunk Game\"],\"Speed\",\"1152×896 <span style=\\\"color: grey;\\\"> ∣ 9:7</span>\",1,\"png\",\"%s\",false,2,4,\"juggernautXL_v8Rundiffusion.safetensors\",\"None\",0.5,true,\"sd_xl_offset_example-lora_1.0.safetensors\",0.1,true,\"None\",1,true,\"None\",1,true,\"None\",1,true,\"None\",1,false,\"uov\",\"Disabled\",null,[],null,\"\",null,false,false,false,false,1.5,0.8,0.3,7,2,\"dpmpp_2m_sde_gpu\",\"karras\",\"Default (model)\",-1,-1,-1,-1,-1,-1,false,false,false,false,64,128,\"joint\",0.25,false,1.01,1.02,0.99,0.95,false,false,\"v2.6\",1,0.618,false,false,0,false,false,\"fooocus\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",false,0,false,null,false,\"Disabled\",\"Before First Enhancement\",\"Original Prompts\",false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false],\"event_data\":null,\"fn_index\":67,\"session_hash\":\"%s\"}"
	initialMessageTemplate := "{\"data\":[null,false,\"%s\",\"\",[\"Fooocus V2\",\"Fooocus Enhance\",\"Fooocus Sharp\"],\"Speed\",\"1152×896 <span style=\\\"color: grey;\\\"> ∣ 9:7</span>\",1,\"png\",\"%s\",false,2,4,\"juggernautXL_v8Rundiffusion.safetensors\",\"None\",0.5,true,\"sd_xl_offset_example-lora_1.0.safetensors\",0.1,true,\"None\",1,true,\"None\",1,true,\"None\",1,true,\"None\",1,false,\"uov\",\"Disabled\",null,[],null,\"\",null,false,false,false,false,1.5,0.8,0.3,7,2,\"dpmpp_2m_sde_gpu\",\"karras\",\"Default (model)\",-1,-1,-1,-1,-1,-1,false,false,false,false,64,128,\"joint\",0.25,false,1.01,1.02,0.99,0.95,false,false,\"v2.6\",1,0.618,false,false,0,false,false,\"fooocus\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",false,0,false,null,false,\"Disabled\",\"Before First Enhancement\",\"Original Prompts\",false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false],\"event_data\":null,\"fn_index\":67,\"session_hash\":\"%s\"}"
	mehaMessageTemplate := "{\"data\":[null,false,\"%s\",\"\",[\"Futuristic Biomechanical Cyberpunk\"],\"Speed\",\"1152×896 <span style=\\\"color: grey;\\\"> ∣ 9:7</span>\",1,\"png\",\"%s\",false,2,4,\"juggernautXL_v8Rundiffusion.safetensors\",\"None\",0.5,true,\"sd_xl_offset_example-lora_1.0.safetensors\",0.1,true,\"None\",1,true,\"None\",1,true,\"None\",1,true,\"None\",1,false,\"uov\",\"Disabled\",null,[],null,\"\",null,false,false,false,false,1.5,0.8,0.3,7,2,\"dpmpp_2m_sde_gpu\",\"karras\",\"Default (model)\",-1,-1,-1,-1,-1,-1,false,false,false,false,64,128,\"joint\",0.25,false,1.01,1.02,0.99,0.95,false,false,\"v2.6\",1,0.618,false,false,0,false,false,\"fooocus\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",false,0,false,null,false,\"Disabled\",\"Before First Enhancement\",\"Original Prompts\",false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false,false,\"\",\"\",\"\",\"sam\",\"full\",\"vit_b\",0.25,0.3,0,false,\"v2.6\",1,0.618,0,false],\"event_data\":null,\"fn_index\":67,\"session_hash\":\"%s\"}"

	randomPart := ""
	for i := 0; i < 20; i++ {
		randomPart += fmt.Sprintf("%d", rand.Int31n(10))
	}

	text := strings.ToLower(update.CallbackQuery.Data)

	var messageTemplate string
	if strings.HasSuffix(text, animeSuffix) || strings.HasSuffix(text, animeSuffixRu) {
		messageTemplate = animeMeassageTemplate
		text, _ = strings.CutSuffix(text, animeSuffix)
		text, _ = strings.CutSuffix(text, animeSuffixRu)
	} else if strings.HasSuffix(text, realisticSuffix) || strings.HasSuffix(text, realisticSuffixRu) {
		messageTemplate = realisticMessageTemplate
		text, _ = strings.CutSuffix(text, realisticSuffix)
		text, _ = strings.CutSuffix(text, realisticSuffixRu)
	} else if strings.HasSuffix(text, cyberpunkSuffix) || strings.HasSuffix(text, cyberpunkSuffixRu) {
		messageTemplate = cyberpunkMessageTemplate
		text, _ = strings.CutSuffix(text, cyberpunkSuffix)
		text, _ = strings.CutSuffix(text, cyberpunkSuffixRu)
	} else if strings.HasSuffix(text, mehaSuffix) || strings.HasSuffix(text, mehaSuffixRu) {
		messageTemplate = mehaMessageTemplate
		text, _ = strings.CutSuffix(text, mehaSuffix)
		text, _ = strings.CutSuffix(text, mehaSuffixRu)
	} else {
		messageTemplate = initialMessageTemplate
	}

	text, _ = strings.CutPrefix(text, "draw ")
	text, _ = strings.CutPrefix(text, "нарисуй ")

	translatedText, err := aiApi.TranslatePrompt(text)
	if err != nil {
		log.Println("Error translating prompt", err)
		processImgGenerationError(ctx, b, update)
		return false
	}

	messageText = fmt.Sprintf(messageTemplate, strings.ReplaceAll(translatedText, "\"", "\\\""), randomPart, hash)
	err = c.WriteMessage(websocket.TextMessage, []byte(messageText))
	defer c.Close()

	if err != nil {
		log.Println("Error sending prompt")
		processImgGenerationError(ctx, b, update)
		return false
	}

	_ = c.ReadJSON(data) // process_starts
	if data.Msg != "process_starts" {
		log.Printf("Error parsing data from server (%s)\n", "process_starts")
		processImgGenerationError(ctx, b, update)
		return false
	}

	_ = c.ReadJSON(data) // process_completed
	if data.Msg != "process_completed" {
		log.Printf("Error parsing data from server (%s)\n", "process_completed")
		processImgGenerationError(ctx, b, update)
		return false
	}

	return true
}
