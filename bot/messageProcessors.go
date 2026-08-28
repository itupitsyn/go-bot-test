package bot

import (
	"bytes"
	"context"
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

func processImageGeneration(ctx context.Context, b *bot.Bot, update *models.Update, mainMessageId int, prompt string) {
	chatId := update.Message.Chat.ID

	processImgGenerationError := func() {
		_, botError := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatId,
			Text:      "Отмена, сервер подох",
			MessageID: mainMessageId,
		})
		utils.ProcessSendMessageError(botError, chatId)
	}

	imageBytes, err := aiApi.GetImage(prompt)
	if err != nil {
		log.Println(err)
		log.Println("[error] error generating image")
		processImgGenerationError()
		return
	}

	photo := &models.InputMediaPhoto{Media: "attach://image.png", MediaAttachment: bytes.NewReader(imageBytes), HasSpoiler: true}
	b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatId,
		MessageID: mainMessageId,
	})

	_, err = b.SendMediaGroup(ctx, &bot.SendMediaGroupParams{
		ChatID: chatId,
		Media:  []models.InputMedia{photo},
		ReplyParameters: &models.ReplyParameters{
			MessageID: update.Message.ID,
		},
	})

	if err != nil {
		processImgGenerationError()
	}
	utils.ProcessSendMessageError(err, chatId)
}

func processVideoGeneration(ctx context.Context, b *bot.Bot, update *models.Update, mainMessageId int, prompt string) {
	chatId := update.Message.Chat.ID

	processVideoGenerationError := func(text string) {
		msgText := text

		if msgText == "" {
			msgText = "Отмена, сервер подох"
		}

		_, botError := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatId,
			Text:      msgText,
			MessageID: mainMessageId,
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

	if imageBytes != nil {
		videoBytes, err = aiApi.GetI2V(prompt, imageBytes, imageName)
	} else {
		videoBytes, err = aiApi.GetT2V(prompt)
	}

	if err != nil {
		log.Println(err)
		log.Println("Error generating i2v")
		processVideoGenerationError("")
		return
	}

	video := &models.InputMediaVideo{Media: "attach://image.mp4", MediaAttachment: bytes.NewReader(videoBytes), HasSpoiler: true}

	b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatId,
		MessageID: mainMessageId,
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

func processTranscription(ctx context.Context, b *bot.Bot, update *models.Update, mainMessageId int) {
	chatId := update.Message.Chat.ID

	processTranscriptionError := func(text string) {
		msgText := text

		if msgText == "" {
			msgText = "Отмена, сервер подох"
		}

		_, botError := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatId,
			Text:      msgText,
			MessageID: mainMessageId,
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

	mediaBytes, mediaName, err := downloadTelegramFile(ctx, b, fileID)
	if err != nil {
		log.Println("Error getting media during transcription")
		log.Println(err)
		processTranscriptionError("")
		return
	}

	text, err := aiApi.GetTranscription(mediaBytes, mediaName)
	if err != nil {
		log.Println(err)
		log.Println("Error transcribing media")
		processTranscriptionError("")
		return
	}

	if strings.TrimSpace(text) == "" {
		processTranscriptionError("Тишина")
		return
	}

	b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatId,
		MessageID: mainMessageId,
	})

	for _, chunk := range utils.SplitText(text, telegramTextLimit) {
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
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
		"<b>Видео</b>\n" +
		"«Анимируй танцующего котика» — бот сделает видео по описанию.\n" +
		"Если отправить картинку с подписью «Анимируй ...» или ответить «Анимируй» на сообщение с картинкой, бот оживит именно её.\n\n" +
		"<b>Расшифровка</b>\n" +
		"Ответь «Расшифруй» на голосовое, кружочек, аудио или видео — бот пришлёт текст.\n" +
		"Можно и сразу: отправь аудио с подписью «Расшифруй».\n\n" +
		"<b>Ответ на сообщение</b>\n" +
		"Ответь на любое текстовое сообщение словом «Нарисуй» или «Анимируй» — промптом станет текст того сообщения.\n" +
		"Всё, что допишешь после команды, добавится к промпту: ответ «Нарисуй аниме» на сообщение «котик на подоконнике» даст «котик на подоконнике аниме».\n\n" +
		"<b>Английский</b>\n" +
		"Всё то же самое: «draw a cat meha», «animate a dancing cat», «transcribe». Стили — anime, realistic, cyberpunk, meha.\n\n" +
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
