package aiApi

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"telebot/utils"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
var realisticSuffix = " realistic"

func sendWaitMessage(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	chatId := update.Message.Chat.ID

	msg := tgbotapi.NewMessage(chatId, "Ладно")
	msg.ReplyToMessageID = update.Message.MessageID
	_, err := bot.Send(msg)
	utils.ProcessSendMessageError(err, chatId)

	time.Sleep(2 * time.Second)

	msg.ReplyToMessageID = 0
	msg.Text = "Жди теперь"
	_, err = bot.Send(msg)
	utils.ProcessSendMessageError(err, chatId)
}

func getWSConnection(bot *tgbotapi.BotAPI, update tgbotapi.Update) (*websocket.Conn, error) {
	chatId := update.Message.Chat.ID
	u := url.URL{Scheme: "ws", Host: os.Getenv("AI_PAINTER_HOST"), Path: "/queue/join"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Println("dial:", err)

		msg := tgbotapi.NewMessage(chatId, "Нет, сервер подох")
		msg.ReplyToMessageID = update.Message.MessageID
		_, err = bot.Send(msg)
		utils.ProcessSendMessageError(err, chatId)
		return nil, err
	}

	return c, nil
}

func initiateImageGeneration(bot *tgbotapi.BotAPI, update tgbotapi.Update, hash string) bool {
	c, err := getWSConnection(bot, update)
	if err != nil {
		return false
	}

	sendWaitMessage(bot, update)

	data := &msgResponse{}
	err = c.ReadJSON(data)
	if data.Msg != "send_hash" {
		log.Printf("Error parsing data from server (%s)\n", "send_hash")
		defer c.Close()
		return false
	}

	messageText := fmt.Sprintf("{\"fn_index\":46,\"session_hash\":\"%s\"}", hash)
	err = c.WriteMessage(websocket.TextMessage, []byte(messageText))
	if err != nil {
		log.Println("Error sending hash")
		defer c.Close()
		return false
	}

	err = c.ReadJSON(data) // estimation
	if data.Msg != "estimation" {
		log.Printf("Error parsing data from server (%s)\n", "estimation")
		defer c.Close()
		return false
	}

	err = c.ReadJSON(data) // send_data
	if data.Msg != "send_data" {
		log.Printf("Error parsing data from server (%s)\n", "send_data")
		defer c.Close()
		return false
	}

	animeMeassageTemplate := "{\"data\":[null,false,\"%s\",\"\",[\"Fooocus V2\",\"Fooocus Semi Realistic\",\"Fooocus Masterpiece\"],\"Speed\",\"896×1152 <span style=\\\"color: grey;\\\"> ∣ 7:9</span>\",1,\"png\",\"5154941903248311516\",false,2,6,\"animaPencilXL_v310.safetensors\",\"None\",0.5,true,\"None\",1,true,\"None\",1,true,\"None\",1,true,\"None\",1,true,\"None\",1,false,\"uov\",\"Disabled\",null,[],null,\"\",null,false,false,false,false,1.5,0.8,0.3,7,2,\"dpmpp_2m_sde_gpu\",\"karras\",\"Default (model)\",-1,-1,-1,-1,-1,-1,false,false,false,false,64,128,\"joint\",0.25,false,1.01,1.02,0.99,0.95,false,false,\"v2.6\",1,0.618,false,false,0,false,\"fooocus\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\"],\"event_data\":null,\"fn_index\":46,\"session_hash\":\"%s\"}"
	realisticMessageTemplate := "{\"data\":[null,false,\"%s\",\"unrealistic, saturated, high contrast, big nose, painting, drawing, sketch, cartoon, anime, manga, render, CG, 3d, watermark, signature, label\",[\"Fooocus V2\",\"Fooocus Photograph\",\"Fooocus Negative\"],\"Speed\",\"896×1152 <span style=\\\"color: grey;\\\"> ∣ 7:9</span>\",1,\"png\",\"2680825016842652469\",false,2,3,\"realisticStockPhoto_v20.safetensors\",\"None\",0.5,true,\"SDXL_FILM_PHOTOGRAPHY_STYLE_BetaV0.4.safetensors\",0.25,true,\"None\",1,true,\"None\",1,true,\"None\",1,true,\"None\",1,false,\"uov\",\"Disabled\",null,[],null,\"\",null,false,false,false,false,1.5,0.8,0.3,7,2,\"dpmpp_2m_sde_gpu\",\"karras\",\"Default (model)\",-1,-1,-1,-1,-1,-1,false,false,false,false,64,128,\"joint\",0.25,false,1.01,1.02,0.99,0.95,false,false,\"v2.6\",1,0.618,false,false,0,false,\"fooocus\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\"],\"event_data\":null,\"fn_index\":46,\"session_hash\":\"%s\"}"
	initialMessageTemplate := "{\"data\":[null,false,\"%s\",\"\",[\"Fooocus V2\",\"Fooocus Enhance\",\"Fooocus Sharp\"],\"Speed\",\"1152×896 <span style=\\\"color: grey;\\\"> ∣ 9:7</span>\",1,\"png\",\"6992129825663615429\",false,2,4,\"juggernautXL_v8Rundiffusion.safetensors\",\"None\",0.5,true,\"sd_xl_offset_example-lora_1.0.safetensors\",0.1,true,\"None\",1,true,\"None\",1,true,\"None\",1,true,\"None\",1,false,\"uov\",\"Disabled\",null,[],null,\"\",null,false,false,false,false,1.5,0.8,0.3,7,2,\"dpmpp_2m_sde_gpu\",\"karras\",\"Default (model)\",-1,-1,-1,-1,-1,-1,false,false,false,false,64,128,\"joint\",0.25,false,1.01,1.02,0.99,0.95,false,false,\"v2.6\",1,0.618,false,false,0,false,\"fooocus\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\",null,0.5,0.6,\"ImagePrompt\"],\"event_data\":null,\"fn_index\":46,\"session_hash\":\"%s\"}"

	text := update.Message.Text

	var messageTemplate string
	if strings.HasSuffix(text, animeSuffix) {
		messageTemplate = animeMeassageTemplate
		text, _ = strings.CutSuffix(text, animeSuffix)
	} else if strings.HasSuffix(text, realisticSuffix) {
		messageTemplate = realisticMessageTemplate
		text, _ = strings.CutSuffix(text, realisticSuffix)
	} else {
		messageTemplate = initialMessageTemplate
	}

	text, _ = strings.CutPrefix(text, "draw ")
	text, _ = strings.CutPrefix(text, "нарисуй ")

	println(text)

	messageText = fmt.Sprintf(messageTemplate, text, hash)
	err = c.WriteMessage(websocket.TextMessage, []byte(messageText))
	if err != nil {
		log.Println("Error sending prompt")
		defer c.Close()
		return false
	}

	err = c.ReadJSON(data) // process_starts
	if data.Msg != "process_starts" {
		log.Printf("Error parsing data from server (%s)\n", "process_starts")
		defer c.Close()
		return false
	}

	err = c.ReadJSON(data) // process_completed
	if data.Msg != "process_completed" {
		log.Printf("Error parsing data from server (%s)\n", "process_completed")
		defer c.Close()
		return false
	}
	defer c.Close()

	return true
}

func processGenerationResult(bot *tgbotapi.BotAPI, update tgbotapi.Update, hash string) {
	c, err := getWSConnection(bot, update)
	defer c.Close()

	if err != nil {
		return
	}

	data := &msgResponse{}
	err = c.ReadJSON(data)
	if data.Msg != "send_hash" {
		log.Printf("Error parsing data from server (%s)\n", "send_hash")
		return
	}

	messageText := fmt.Sprintf("{\"fn_index\":47,\"session_hash\":\"%s\"}", hash)
	err = c.WriteMessage(websocket.TextMessage, []byte(messageText))
	if err != nil {
		log.Println("Error sending hash")
		return
	}

	err = c.ReadJSON(data) // estimation
	if data.Msg != "estimation" {
		log.Printf("Error parsing data from server (%s)\n", "estimation")
		return
	}

	err = c.ReadJSON(data) // send_data
	if data.Msg != "send_data" {
		log.Printf("Error parsing data from server (%s)\n", "send_data")
		return
	}

	messageText = fmt.Sprintf("{\"data\":[null],\"event_data\":null,\"fn_index\":47,\"session_hash\":\"%s\"}", hash)
	err = c.WriteMessage(websocket.TextMessage, []byte(messageText))
	if err != nil {
		log.Println("Error sending data for getting result")
		return
	}

	processData := &msgImageGeneratedRespons{}
	err = c.ReadJSON(processData) // process_starts, process_generating, process_completed
	if processData.Msg != "process_starts" {
		log.Printf("Error parsing data from server (%s)\n", "process_starts")
		return
	}

	for processData.Msg != "process_completed" {
		err = c.ReadJSON(processData)
		if processData.Msg != "process_generating" && processData.Msg != "process_completed" {
			log.Printf("Error parsing data from server (%s)\n", "process_generating/process_completed")
			return
		}
	}

	if len(processData.Output.Data) < 4 || len(processData.Output.Data[3].Value) < 1 {
		log.Println("Error parsing generation result")
		return
	}

	pathToImage := processData.Output.Data[3].Value[0].Name
	url := fmt.Sprintf("http://%s/file=%s", os.Getenv("AI_PAINTER_HOST"), pathToImage)

	response, e := http.Get(url)
	if e != nil {
		defer response.Body.Close()
		log.Println("Error getting generated image")
		return
	}

	defer response.Body.Close()

	imageBytes, err := io.ReadAll(response.Body)
	if e != nil {
		log.Println("Error reading generated image")
		defer response.Body.Close()
		return
	}

	photoFileBytes := tgbotapi.FileBytes{
		Name:  "picture",
		Bytes: imageBytes,
	}

	chatId := update.Message.Chat.ID

	msg := tgbotapi.NewPhoto(chatId, photoFileBytes)

	from := update.Message.From
	if from.UserName != "" {
		msg.Caption = fmt.Sprintf("@%s", from.UserName)
	} else {
		msg.ParseMode = "HTML"
		msg.Caption = fmt.Sprintf("<a href=\"tg://user?id=%d\">%s</a>", from.ID, utils.GetAlternativeName(from))
	}

	_, err = bot.Send(msg)
	utils.ProcessSendMessageError(err, chatId)

}

func GetImage(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	hash := string(uuid.NewString()[:8])
	success := initiateImageGeneration(bot, update, hash)
	if !success {
		return
	}
	processGenerationResult(bot, update, hash)
}