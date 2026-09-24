package bot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"telebot/aiApi"
	"telebot/model"
	"telebot/raffleLogic"
	"telebot/utils"
	"time"
	"unicode/utf8"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"gorm.io/datatypes"
)

func processStats(ctx context.Context, b *bot.Bot, update *models.Update, full bool) {
	var stats *[]model.Stats
	if full {
		stats = model.GetFullStats(update.Message.Chat.ID)
	} else {
		stats = model.GetStats(update.Message.Chat.ID)
	}

	var msgText string
	if len(*stats) == 0 {
		msgText = "Статистики еще нет. Здеся"
	} else {
		maxNameLen := 17
		maxCountsLen := 4
		msgText = "<code>"
		msgText += fmt.Sprintf("%-*s %*s\n", maxNameLen, "winner", maxCountsLen, "wins")
		for _, current := range *stats {
			var currentName string
			if current.Name != "" {
				currentName = current.Name
			} else {
				currentName = current.Alternativename
			}
			if utf8.RuneCountInString(currentName) > maxNameLen {
				currentName = currentName[:maxNameLen-2] + ".."
			}
			msgText += fmt.Sprintf("%-*s %*d\n", maxNameLen, currentName, maxCountsLen, current.Count)
		}
		msgText += "</code>"
	}

	chatId := update.Message.Chat.ID
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatId,
		Text:      msgText,
		ParseMode: models.ParseModeHTML,
	})
	utils.ProcessSendMessageError(err, chatId)
}

func processParticipation(update *models.Update) {
	from := update.Message.From
	name := from.Username
	alternativeName := utils.GetAlternativeName(from)

	if name != "" {
		log.Println("Participation requested by", name)
	} else {
		log.Println("Participation requested by", alternativeName)
	}

	usr := model.User{
		ID:              from.ID,
		Name:            name,
		AlternativeName: alternativeName,
	}

	raffle := model.Raffle{
		ChatID:       update.Message.Chat.ID,
		Date:         datatypes.Date(time.Now().In(time.UTC)),
		Participants: []model.User{},
	}
	raffle.Save()

	participants := model.Raffle{
		ChatID: update.Message.Chat.ID,
		Date:   datatypes.Date(time.Now().In(time.UTC)),
		Participants: []model.User{
			usr,
		},
	}
	participants.Save()
}

func processImageGeneration(ctx context.Context, b *bot.Bot, update *models.Update, wait *waitMessage, prompt string) {
	chatId := update.Message.Chat.ID

	processImgGenerationError := func(text string) {
		wait.done()

		_, botError := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatId,
			Text:      text,
			MessageID: wait.id(),
		})
		utils.ProcessSendMessageError(botError, chatId)
	}

	imageBytes, err := aiApi.GetImage(prompt, messageCaller(update.Message, wait))
	wait.done()
	if err != nil {
		log.Println(err)
		log.Println("[error] error generating image")
		processImgGenerationError(generationErrorText(err))
		return
	}

	photo := &models.InputMediaPhoto{Media: "attach://image.png", MediaAttachment: bytes.NewReader(imageBytes), HasSpoiler: true}
	b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatId,
		MessageID: wait.id(),
	})

	_, err = b.SendMediaGroup(ctx, &bot.SendMediaGroupParams{
		ChatID: chatId,
		Media:  []models.InputMedia{photo},
		ReplyParameters: &models.ReplyParameters{
			MessageID: update.Message.ID,
		},
	})

	if err != nil {
		processImgGenerationError(serverDeadText)
	}
	utils.ProcessSendMessageError(err, chatId)
}

// editPhotos collects the images to edit: first the one being replied to, then
// the one attached to the message itself.
//
// The order is no accident: people usually reply to the source and attach what
// they want to add to it, and the service treats several images as a mix.
// Telegram puts the sizes of ONE photo into Photo, so there can't be more than
// two here.
func editPhotos(message *models.Message) []*models.PhotoSize {
	var out []*models.PhotoSize

	if reply := message.ReplyToMessage; reply != nil {
		if img := getBiggestPhoto(reply.Photo); img != nil {
			out = append(out, img)
		}
	}
	if img := getBiggestPhoto(message.Photo); img != nil {
		out = append(out, img)
	}

	return out
}

// editHintText is the answer to "нарисуй" with an image but no instruction.
//
// There is nothing to guess here: an edit has no meaningful default, unlike
// animation, where the image can simply be brought to life.
const editHintText = "Напиши, что поправить: «нарисуй ей рыжие волосы», " +
	"«нарисуй зимнюю улицу вместо фона»."

// processEditHint tells what the command was missing.
func processEditHint(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := update.Message.Chat.ID
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatId,
		Text:            editHintText,
		ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
	})
	utils.ProcessSendMessageError(err, chatId)
}

// processImageEdit edits the given images following an instruction. We get here
// when an image is attached to the "нарисуй" command or the command replies to
// an image.
func processImageEdit(ctx context.Context, b *bot.Bot, update *models.Update, wait *waitMessage, prompt string, photos []*models.PhotoSize) {
	chatId := update.Message.Chat.ID

	processEditError := func(text string) {
		wait.done()

		msgText := text
		if msgText == "" {
			msgText = serverDeadText
		}

		_, botError := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatId,
			Text:      msgText,
			MessageID: wait.id(),
		})
		utils.ProcessSendMessageError(botError, chatId)
	}

	images := make([]aiApi.EditImage, 0, len(photos))
	for _, photo := range photos {
		imageBytes, imageName, err := downloadTelegramFile(ctx, b, photo.FileID)
		if err != nil {
			log.Println("Error getting image during edit")
			log.Println(err)
			processEditError("")
			return
		}
		images = append(images, aiApi.EditImage{Bytes: imageBytes, Name: imageName})
	}

	imageBytes, err := aiApi.GetImageEdit(prompt, images, messageCaller(update.Message, wait))
	wait.done()
	if err != nil {
		log.Println(err)
		log.Println("[error] error editing image")
		processEditError(generationErrorText(err))
		return
	}

	photo := &models.InputMediaPhoto{Media: "attach://image.png", MediaAttachment: bytes.NewReader(imageBytes), HasSpoiler: true}
	b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatId,
		MessageID: wait.id(),
	})

	_, err = b.SendMediaGroup(ctx, &bot.SendMediaGroupParams{
		ChatID: chatId,
		Media:  []models.InputMedia{photo},
		ReplyParameters: &models.ReplyParameters{
			MessageID: update.Message.ID,
		},
	})

	if err != nil {
		processEditError(serverDeadText)
	}
	utils.ProcessSendMessageError(err, chatId)
}

func processVideoGeneration(ctx context.Context, b *bot.Bot, update *models.Update, wait *waitMessage, prompt string) {
	chatId := update.Message.Chat.ID

	processVideoGenerationError := func(text string) {
		wait.done()

		msgText := text

		if msgText == "" {
			msgText = serverDeadText
		}

		_, botError := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatId,
			Text:      msgText,
			MessageID: wait.id(),
		})
		utils.ProcessSendMessageError(botError, chatId)
	}

	imgs := update.Message.Photo
	if len(imgs) == 0 {
		reply := update.Message.ReplyToMessage
		if reply != nil {
			log.Printf("Video: reply present, reply.Photo=%d, reply.Animation=%v, reply.Video=%v, reply.Document=%v",
				len(reply.Photo), reply.Animation != nil, reply.Video != nil, reply.Document != nil)
			if len(reply.Photo) > 0 {
				imgs = reply.Photo
			}
		} else {
			log.Println("Video: no photo in message and no reply")
		}
	}

	var imageName string
	var imageBytes []byte

	if img := getBiggestPhoto(imgs); img != nil {
		var err error
		imageBytes, imageName, err = downloadTelegramFile(ctx, b, img.FileID)
		if err != nil {
			log.Println("Error getting image during I2V generation")
			log.Println(err)
			processVideoGenerationError("")
			return
		}
	}

	if prompt == "" {
		prompt = defaultAnimationPrompt
	}

	var videoBytes []byte
	var err error

	caller := messageCaller(update.Message, wait)
	if imageBytes != nil {
		videoBytes, err = aiApi.GetI2V(prompt, imageBytes, imageName, caller)
	} else {
		videoBytes, err = aiApi.GetT2V(prompt, caller)
	}
	wait.done()

	if err != nil {
		log.Println(err)
		log.Println("Error generating i2v")
		processVideoGenerationError(generationErrorText(err))
		return
	}

	video := &models.InputMediaVideo{Media: "attach://image.mp4", MediaAttachment: bytes.NewReader(videoBytes), HasSpoiler: true}

	b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatId,
		MessageID: wait.id(),
	})

	_, err = b.SendMediaGroup(ctx, &bot.SendMediaGroupParams{
		ChatID: chatId,
		Media:  []models.InputMedia{video},
		ReplyParameters: &models.ReplyParameters{
			MessageID: update.Message.ID,
		},
	})

	if err != nil {
		processVideoGenerationError("")
	}
	utils.ProcessSendMessageError(err, chatId)
}

// telegramTextLimit is the longest text Telegram accepts in a single message.
const telegramTextLimit = 4096

func processTranscription(ctx context.Context, b *bot.Bot, update *models.Update, wait *waitMessage) {
	chatId := update.Message.Chat.ID

	processTranscriptionError := func(text string) {
		wait.done()

		msgText := text

		if msgText == "" {
			msgText = serverDeadText
		}

		_, botError := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatId,
			Text:      msgText,
			MessageID: wait.id(),
		})
		utils.ProcessSendMessageError(botError, chatId)
	}

	// The audio either rides along with the command or sits in the message the
	// command replies to; in the latter case the text belongs under the
	// original, not under the command.
	fileID := getTranscribableFileID(update.Message)
	replyToId := update.Message.ID
	if fileID == "" {
		if reply := update.Message.ReplyToMessage; reply != nil {
			if replyFileID := getTranscribableFileID(reply); replyFileID != "" {
				fileID = replyFileID
				replyToId = reply.ID
			}
		}
	}

	if fileID == "" {
		log.Println("Transcription: no audio or video in message and no reply carrying one")
		processTranscriptionError("Нечего расшифровывать")
		return
	}

	text, failText := transcribeFile(ctx, b, fileID, messageCaller(update.Message, wait))
	wait.done()
	if failText != "" {
		processTranscriptionError(failText)
		return
	}

	b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatId,
		MessageID: wait.id(),
	})

	for _, chunk := range utils.SplitText(text, telegramTextLimit) {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatId,
			Text:            chunk,
			ReplyParameters: &models.ReplyParameters{MessageID: replyToId},
		})
		utils.ProcessSendMessageError(err, chatId)
		if err != nil {
			return
		}
	}
}

// transcribeFile downloads an audio or video from Telegram and sends it for
// transcription. The second value is what to say in the chat instead of the
// text; empty means there is text.
func transcribeFile(ctx context.Context, b *bot.Bot, fileID string, caller aiApi.Caller) (string, string) {
	mediaBytes, mediaName, err := downloadTelegramFile(ctx, b, fileID)
	if err != nil {
		log.Println("Error getting media during transcription")
		log.Println(err)

		// A file that is too big is not a dead server but Telegram's limit:
		// blaming the server here would invite the person to try again when
		// there is nothing to retry.
		if errors.Is(err, ErrFileTooBig) {
			return "", fileTooBigText
		}

		return "", serverDeadText
	}

	text, err := aiApi.GetTranscription(mediaBytes, mediaName, caller)
	if err != nil {
		log.Println(err)
		log.Println("Error transcribing media")
		return "", generationErrorText(err)
	}

	if strings.TrimSpace(text) == "" {
		return "", "Тишина"
	}

	return text, ""
}

// processSummary retells the message the command replies to. Unlike drawing or
// transcribing this only costs an llm call, so it goes without the "Жди теперь"
// song and dance, just one short "занят" while the llm thinks. Audio and video
// get transcribed first, still under the same "занят": for the one who asked it
// is all one retelling.
func processSummary(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := update.Message.Chat.ID

	reply := func(text string, replyToId int) {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatId,
			Text:            text,
			ReplyParameters: &models.ReplyParameters{MessageID: replyToId},
		})
		utils.ProcessSendMessageError(err, chatId)
	}

	// Media with "что тут" in the caption is an explicit request to the bot,
	// just like with "расшифруй". Otherwise a command that replies to nothing
	// is just a remark in the chat, not a request to the bot: stay silent so as
	// not to litter with hints.
	source := update.Message
	fileID := getTranscribableFileID(source)
	if fileID == "" {
		source = update.Message.ReplyToMessage
		if source == nil {
			return
		}
		// A voice message or video is transcribed first and then retold: asking
		// "расшифруй" and then "что тут" on the transcription is a needless
		// extra round.
		fileID = getTranscribableFileID(source)
	}

	// There is nothing to retell for images without a caption and for stickers:
	// getMessageText is empty for them. A media caption is not retold: the
	// point is what was said, and the caption is often the command itself.
	text := ""
	if fileID == "" {
		text = strings.TrimSpace(getMessageText(source))
		if text == "" {
			return
		}
	}

	// Maintenance is checked only now: the bot stays silent on a bare "что тут"
	// that replies to nothing, and there is no point talking about repairs
	// there either.
	if replyIfMaintenance(ctx, b, update.Message) {
		return
	}

	// A retelling takes a few seconds, and staying silent through them won't
	// do: it's unclear whether the request was heard at all. The place in the
	// queue is not shown here even when the transcription waits in it: for the
	// one asking, a retelling is one short request, not a generation.
	waitMsg, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatId,
		Text:            summaryWaitText,
		ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
	})
	utils.ProcessSendMessageError(err, chatId)

	// A failure goes into the same message so as not to multiply them in the
	// chat. If sending "Ща, сек" failed, answer with a regular reply.
	fail := func(reason string) {
		if waitMsg == nil {
			reply(reason, update.Message.ID)
			return
		}

		_, botError := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatId,
			MessageID: waitMsg.ID,
			Text:      reason,
		})
		utils.ProcessSendMessageError(botError, chatId)
	}

	if fileID != "" {
		failText := ""
		text, failText = transcribeFile(ctx, b, fileID, messageCaller(update.Message, nil))
		if failText != "" {
			fail(failText)
			return
		}
	}

	// The client language of whoever asked, not of whoever wrote the original
	// message: the retelling is for the asker to read. Telegram doesn't always
	// send this field.
	languageCode := ""
	if update.Message.From != nil {
		languageCode = update.Message.From.LanguageCode
	}

	summary, err := aiApi.GetSummary(text, languageCode)
	if err != nil {
		log.Println("[error] error summarizing a message")
		log.Println(err)
		fail(serverDeadText)
		return
	}

	if strings.TrimSpace(summary) == "" {
		fail("Нечего сказать")
		return
	}

	if waitMsg != nil {
		b.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    chatId,
			MessageID: waitMsg.ID,
		})
	}

	// The retelling is attached to the original message so it reads under it.
	for _, chunk := range utils.SplitText(summary, telegramTextLimit) {
		reply(chunk, source.ID)
	}
}

func processPrize(ctx context.Context, b *bot.Bot, update *models.Update, chat *model.Chat) {
	chatId := update.Message.Chat.ID
	user := model.User{
		ID: update.Message.From.ID,
	}

	phraseParts := strings.Split(strings.ToLower(update.Message.Text), " ")
	if len(phraseParts) < 2 {
		return
	}

	if !user.CanCreatePrize(chatId) {
		phrazes := raffleLogic.GetRandomPhrazeByKey(raffleLogic.WrongAdminKey, chat.IsUncensored)
		go utils.SendPhrazes(ctx, b, chat, phrazes, update.Message.ID)

		return
	}

	var date datatypes.Date
	if phraseParts[0] == "сегодня" {
		if raffleLogic.IsNoReturnPoint() {
			phrazes := raffleLogic.GetRandomPhrazeByKey(raffleLogic.TooLateKey, chat.IsUncensored)
			if len(phrazes) > 0 {
				go utils.SendPhrazes(ctx, b, chat, phrazes, update.Message.ID)
			} else {
				_, err := b.SendMessage(ctx, &bot.SendMessageParams{
					ChatID:          chatId,
					Text:            "ПОЗДНО!",
					ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
				})
				utils.ProcessSendMessageError(err, chatId)
			}
			return
		} else {
			date = datatypes.Date(time.Now().In(time.UTC))
		}
	} else {
		date = datatypes.Date(time.Now().In(time.UTC).AddDate(0, 0, 1))
	}
	phraseParts = strings.Split(update.Message.Text, " ")
	newPrize := strings.Join(phraseParts[1:], " ")

	model.DeletePrizeByDate(date, chatId)
	prize := model.Prize{
		Name:   newPrize,
		ChatID: chatId,
		Date:   date,
	}
	prize.Save()

	phrazes := raffleLogic.GetRandomPhrazeByKey(raffleLogic.AcceptPrizeKey, chat.IsUncensored)
	go utils.SendPhrazes(ctx, b, chat, phrazes, update.Message.ID)
}

func processPrizeInfo(ctx context.Context, b *bot.Bot, chat *model.Chat) {
	chatId := chat.ID
	year, month, day := time.Now().In(time.UTC).Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)

	dates := []datatypes.Date{datatypes.Date(today), datatypes.Date(today.AddDate(0, 0, 1))}
	prizes, _ := model.GetPrizesByDate(dates, chatId)

	prizeToday := raffleLogic.GetPrizeName(nil, chat)
	prizeTomorrow := raffleLogic.GetPrizeName(nil, chat)
	for _, prize := range *prizes {
		if prize.Date == dates[0] {
			prizeToday = raffleLogic.GetPrizeName(&prize, chat)
		} else {
			prizeTomorrow = raffleLogic.GetPrizeName(&prize, chat)
		}
	}

	msgText := fmt.Sprintf("Сегодня — %s \nЗавтра — %s", prizeToday, prizeTomorrow)
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatId,
		Text:   msgText,
	})

	utils.ProcessSendMessageError(err, chatId)
}

func checkSetCommandInitiator(ctx context.Context, b *bot.Bot, update *models.Update) error {
	initiatorUserId := update.Message.From.ID
	chatId := update.Message.Chat.ID

	if ok, saError := model.IsSuperAdmin(chatId, initiatorUserId); !ok {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatId,
			Text:   "Запрещено!",
		})
		utils.ProcessSendMessageError(err, chatId)
		return saError
	}

	return nil
}

func getSetCommandUserID(ctx context.Context, b *bot.Bot, update *models.Update) (int64, error) {
	chatId := update.Message.Chat.ID
	command := update.Message.Text
	parts := strings.Split(command, " ")
	if len(parts) < 2 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatId,
			Text:   "Неверная команда",
		})
		utils.ProcessSendMessageError(err, chatId)
		return 0, fmt.Errorf("invalid command: %s", command)
	}
	userName := strings.Trim(parts[1], "@ ")
	user, userByNameErr := model.GetUserByName(userName)
	if userByNameErr != nil {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatId,
			Text:   "Такого члена чята нет!",
		})
		utils.ProcessSendMessageError(err, chatId)
		return 0, userByNameErr
	}
	// TODO: We should also check if mentioned user is actually a member of our chat
	return user.ID, nil
}

func setUserRoleViaCommand(ctx context.Context, b *bot.Bot, userId int64, chatId int64, roleID int64) error {
	chatUserRole, firstChatUserError := model.GetFirstChatUserRole(chatId, userId)

	if firstChatUserError != nil {
		log.Println("No role found, creating new one", firstChatUserError)
		chatUserRole := model.ChatUserRole{
			ChatID:        chatId,
			UserID:        userId,
			RoleID:        roleID,
			IsSetManually: true,
		}
		if _, err := chatUserRole.Save(); err != nil {
			log.Println("[error] error creating role", err)
			return err
		}
	} else {
		if chatUserRole.RoleID == model.SuperAdminRoleID {
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatId,
				Text:   "Ты гэта не трогай його!",
			})
			utils.ProcessSendMessageError(err, chatId)
			return fmt.Errorf("user %d is super admin", userId)
		}
		chatUserRole.RoleID = roleID
		if _, err := chatUserRole.Save(); err != nil {
			log.Println("[error] error creating role", err)
			return err
		}
	}
	return nil
}

func processSetAdmin(ctx context.Context, b *bot.Bot, update *models.Update) {
	if checkSetCommandInitiator(ctx, b, update) != nil {
		return
	}
	chatId := update.Message.Chat.ID
	userId, err := getSetCommandUserID(ctx, b, update)
	if err != nil {
		return
	}
	err = setUserRoleViaCommand(ctx, b, userId, chatId, model.PrizeCreatorRoleID)
	if err != nil {
		return
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatId,
		Text:   "Одминка выдана!",
	})

	utils.ProcessSendMessageError(err, chatId)
}

func processUnsetAdmin(ctx context.Context, b *bot.Bot, update *models.Update) {
	if checkSetCommandInitiator(ctx, b, update) != nil {
		return
	}
	chatId := update.Message.Chat.ID
	userId, err := getSetCommandUserID(ctx, b, update)
	if err != nil {
		return
	}
	err = setUserRoleViaCommand(ctx, b, userId, chatId, model.PlayerRoleID)
	if err != nil {
		return
	}
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatId,
		Text:   "Одминка отобрана!",
	})
	utils.ProcessSendMessageError(err, chatId)
}

func processAdmins(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := update.Message.Chat.ID

	// TODO: Proper solution is left join, but it's time consuming to implement

	chatUserRoles, chatAdminsError := model.GetChatAdmins(chatId)

	if chatAdminsError != nil || len(chatUserRoles) == 0 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatId,
			Text:   "Админов нет!",
		})
		utils.ProcessSendMessageError(err, chatId)
		return
	}
	log.Printf("%#v", chatUserRoles)

	msgText := "<code>\n"
	msgText += "Одмины:\n"
	var user_ids []int64 = []int64{}
	for _, chatUserRole := range chatUserRoles {
		user_ids = append(user_ids, chatUserRole.UserID)
	}
	users := model.GetUsersByIds(user_ids)

	userByUserID := make(map[int64]model.User)
	for _, user := range users {
		userByUserID[user.ID] = user
	}

	for _, chatUserRole := range chatUserRoles {
		roleID := chatUserRole.RoleID
		user := userByUserID[chatUserRole.UserID]
		var role string = "хз кто"
		if roleID == model.SuperAdminRoleID {
			role = "superadmin"
		} else if roleID == model.PrizeCreatorRoleID {
			role = "admin"
		}
		msgText += fmt.Sprintf("%s (%s)\n", user.Name, role)
	}
	msgText += "</code>"

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatId,
		Text:      msgText,
		ParseMode: models.ParseModeHTML,
	})
	utils.ProcessSendMessageError(err, chatId)
}

func processAIHelp(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := update.Message.Chat.ID
	msgText := "<b>Картинки</b>\n" +
		"Напиши «Нарисуй котика» — бот пришлёт картинку. Работает и в группе, и в личке.\n\n" +
		"<b>Стили</b>\n" +
		"Допиши в конце аниме, реалистично, киберпанк или меха — картинка будет в этой стилистике.\n" +
		"Например: «Нарисуй котика киберпанк».\n\n" +
		"<b>Правка картинок</b>\n" +
		"Та же команда, но с картинкой — и бот не нарисует новую, а поправит присланную.\n" +
		"Ответь «Нарисуй ей рыжие волосы» на сообщение с фото — или отправь фото с такой подписью.\n" +
		"Пиши, что сделать, а не что нарисовать: «нарисуй зимнюю улицу вместо фона», «нарисуй ему бороду».\n" +
		"Всё, кроме правки, останется как было — лицо, поза и фон не поедут.\n" +
		"Если приложить своё фото в ответ на чужое, бот смешает оба: «нарисуй её за этим столом».\n\n" +
		"<b>Видео</b>\n" +
		"«Анимируй танцующего котика» — бот сделает видео по описанию.\n" +
		"Если отправить картинку с подписью «Анимируй ...» или ответить «Анимируй» на сообщение с картинкой, бот оживит именно её.\n\n" +
		"<b>Движение камеры</b>\n" +
		"Допиши в конце медленно, вокруг, ближе, дальше, сверху или живо — и камера поведёт себя соответственно.\n" +
		"Например: «Анимируй котика вокруг» — камера обойдёт его по кругу.\n" +
		"Видео получается со звуком, он подбирается под сцену сам.\n\n" +
		"<b>Расшифровка</b>\n" +
		"Ответь «Расшифруй» на голосовое, кружочек, аудио или видео — бот пришлёт текст.\n" +
		"Можно и сразу: отправь аудио с подписью «Расшифруй».\n\n" +
		"<b>Пересказ</b>\n" +
		"Ответь «Что тут» или «Сократи» на длинное сообщение — бот перескажет его в паре предложений.\n" +
		"На голосовое, кружочек, аудио или видео — тоже: бот сам расшифрует и сразу перескажет.\n" +
		"Отвечает на языке твоего Telegram.\n\n" +
		"<b>Ответ на сообщение</b>\n" +
		"Ответь на любое текстовое сообщение словом «Нарисуй» или «Анимируй» — промптом станет текст того сообщения.\n" +
		"Всё, что допишешь после команды, добавится к промпту: ответ «Нарисуй аниме» на сообщение «котик на подоконнике» даст «котик на подоконнике аниме».\n\n" +
		"<b>Английский</b>\n" +
		"Всё то же самое: «draw a cat meha», «animate a dancing cat around», «transcribe», «summarize».\n" +
		"Стили — anime, realistic, cyberpunk, meha. Камера — slowly, around, closer, wider, above, lively.\n\n" +
		"<b>Inline-режим</b>\n" +
		fmt.Sprintf("Набери в любом чате @%s и промпт — добавлять меня в этот чат не нужно.\n", botName) +
		"«Что рисуем?» — картинка, «Что анимируем?» — видео по описанию.\n\n" +
		"<b>Своя картинка в inline</b>\n" +
		"Пришли мне картинку в личку — и в inline-результатах появится пункт «Анимировать мою картинку». Работает в любом чате, даже если меня там нет.\n" +
		"Промптом станет то, что набрано после имени бота; не набрано ничего — оживлю как есть.\n" +
		inlineImageBufferHint()
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatId,
		Text:            msgText,
		ParseMode:       models.ParseModeHTML,
		ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
	})
	utils.ProcessSendMessageError(err, chatId)
}

func processHelp(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := update.Message.Chat.ID
	msgText := "Привет, странник! Я разнообразю серые будни групповых чатов ежедневными розыгрышами. Добавь меня в чат — и понеслось.\n\n" +
		"<b>Как проходит розыгрыш</b>\n" +
		"Каждый день в 12:00 UTC я выбираю случайного победителя. Участвуют все, кто с полуночи UTC и до розыгрыша написал в чат хотя бы одно сообщение. Если желающих меньше двух, розыгрыш не проводится — скучно.\n\n" +
		"<b>Приз</b>\n" +
		"По умолчанию разыгрывается обыденное ничего, но приз можно назначить свой: напиши «Сегодня развесёлое ничего» или «Завтра волшебное ничего».\n" +
		"Приз на сегодня меняется только до 12:00 UTC, после розыгрыша поезд ушёл.\n" +
		"/prize — что разыгрывается сегодня и завтра.\n\n" +
		"<b>Кто может менять приз</b>\n" +
		"Суперадмины — это владелец чата и его админы, я нахожу их сам. Ещё они могут выдать одминку кому угодно:\n" +
		"/set_admin @user — выдать\n" +
		"/unset_admin @user — отобрать\n" +
		"/admins — посмотреть, кто в списке\n\n" +
		"<b>Статистика</b>\n" +
		"/stats — победители с начала года\n" +
		"/stats_full — победители за всё время\n\n" +
		"<b>Ещё я рисую, анимирую и расшифровываю голосовые</b>\n" +
		"/ai_help — как этим пользоваться."
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatId,
		Text:            msgText,
		ParseMode:       models.ParseModeHTML,
		ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
	})
	utils.ProcessSendMessageError(err, chatId)
}

func saveUser(from *models.User) {
	name := from.Username
	alternativeName := utils.GetAlternativeName(from)

	usr := model.User{
		ID:              from.ID,
		Name:            name,
		AlternativeName: alternativeName,
	}
	usr.Save()
}

func saveChat(update *models.Update) (*model.Chat, error) {
	chatId := update.Message.Chat.ID

	chat, err := model.GetChatById(chatId)
	if err == nil {
		return chat, nil
	}

	chat = &model.Chat{
		ID:           update.Message.Chat.ID,
		Name:         update.Message.Chat.Title,
		IsUncensored: false,
	}
	_, err = chat.Save()
	if err != nil {
		log.Println("error saving chat ", err)
		return nil, err
	}

	return chat, nil
}

func getUserFromChatMember(chatAdmin *models.ChatMember) *models.User {
	var user *models.User
	if chatAdmin.Owner != nil {
		user = chatAdmin.Owner.User
	} else if chatAdmin.Administrator != nil {
		user = &chatAdmin.Administrator.User
	}

	return user
}

func syncSuperAdmins(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := update.Message.Chat.ID
	chatAdmins, err := b.GetChatAdministrators(ctx, &bot.GetChatAdministratorsParams{ChatID: chatId})
	if err != nil {
		log.Println("Error getting chat admins", err)
	}
	userRoles, err := model.GetChatAdmins(chatId)
	if err != nil {
		log.Println("Error getting chat roles", err)
	}

	// remove old admins
	for _, userRole := range userRoles {
		if userRole.IsSetManually || userRole.RoleID != model.SuperAdminRoleID {
			continue
		}
		isExtra := true
		for _, chatMemeber := range chatAdmins {
			user := getUserFromChatMember(&chatMemeber)
			if user != nil && user.ID == userRole.UserID {
				isExtra = false
				break
			}
		}

		if isExtra {
			err := userRole.DeleteChatUserRole()
			if err != nil {
				log.Println("Error deleting roles", err)
			} else {
				log.Println("Superadmin is automatically removed", userRole.UserID, userRole.ChatID)
			}
		}
	}

	// add new admins
	for _, chatMember := range chatAdmins {
		user := getUserFromChatMember(&chatMember)
		if user == nil || user.IsBot {
			continue
		}

		doesContain := false
		for _, roleAdmin := range userRoles {
			if roleAdmin.UserID == user.ID && (roleAdmin.RoleID == model.SuperAdminRoleID || roleAdmin.IsSetManually) {
				doesContain = true
				break
			}
		}
		if doesContain {
			continue
		}

		newAdmin := model.ChatUserRole{
			UserID: user.ID,
			ChatID: chatId,
			RoleID: model.SuperAdminRoleID,
		}
		newAdmin.Save()
		log.Println("Superadmin is automatically set", utils.GetAnyName(user), update.Message.Chat.Title)
	}
}
