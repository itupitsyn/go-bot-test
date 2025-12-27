package bot

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
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
		Date:         datatypes.Date(time.Now()),
		Participants: []model.User{},
	}
	raffle.Save()

	participants := model.Raffle{
		ChatID: update.Message.Chat.ID,
		Date:   datatypes.Date(time.Now()),
		Participants: []model.User{
			usr,
		},
	}
	participants.Save()
}

func processImageGeneration(ctx context.Context, b *bot.Bot, update *models.Update, mainMessageId int) {
	chatId := update.Message.Chat.ID

	processImgGenerationError := func() {
		_, botError := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatId,
			Text:      "Отмена, сервер подох",
			MessageID: mainMessageId,
		})
		utils.ProcessSendMessageError(botError, chatId)
	}

	imageBytes, err := aiApi.GetImage(update.Message.Text)
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

func processI2VGeneration(ctx context.Context, b *bot.Bot, update *models.Update, mainMessageId int) {
	chatId := update.Message.Chat.ID

	processI2VGenerationError := func(text string) {
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
		if update.Message.ReplyToMessage != nil && len(update.Message.ReplyToMessage.Photo) > 0 {
			imgs = update.Message.ReplyToMessage.Photo
		} else {
			processI2VGenerationError("Братюнь, нужна картинка")
			return
		}
	}

	maxSizeImg := imgs[0]

	for _, img := range imgs {
		if maxSizeImg.FileSize < img.FileSize {
			maxSizeImg = img
		}
	}

	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: maxSizeImg.FileID})
	if err != nil {
		log.Println("Error getting image by id during I2V generation")
		processI2VGenerationError("")
		return
	}

	downloadURL := b.FileDownloadLink(file)

	resp, err := http.Get(downloadURL)
	if err != nil {
		log.Println("Error downloading image by id during I2V generation")
		processI2VGenerationError("")
		return
	}
	defer resp.Body.Close()

	imageBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error getting image bytes during I2V generation")
		processI2VGenerationError("")
		return
	}

	imgName := filepath.Base(file.FilePath)

	msgText := update.Message.Caption
	if msgText == "" {
		msgText = update.Message.Text
	}

	prompt := utils.TrimPrefixIgnoreCase(msgText, "анимируй")
	prompt = utils.TrimPrefixIgnoreCase(prompt, "animate")

	videoBytes, err := aiApi.GetI2V(prompt, imageBytes, imgName)
	if err != nil {
		log.Println(err)
		log.Println("Error generating i2v")
		processI2VGenerationError("")
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
		processI2VGenerationError("")
	}
	utils.ProcessSendMessageError(err, chatId)
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
		go utils.SendPhrazes(ctx, b, chat, phrazes)

		return
	}

	var date datatypes.Date
	if phraseParts[0] == "сегодня" {
		if raffleLogic.IsNoReturnPoint() {
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatId,
				Text:   "ПОЗДНО!",
			})
			utils.ProcessSendMessageError(err, chatId)
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
	go utils.SendPhrazes(ctx, b, chat, phrazes)
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
	msgText := "Ты можешь отправить сообщение, например, \"Нарисуй котика\", и бот сгенерирует изображение котика. Сообщение можно отправить как в групповом чате, так и в личке! Если в конце будут слова аниме, реалистично, киберпанк или меха, то изображение будет в соответствующей стилистике.\n\nМожно писать на английском, например draw a cat meha\nВозможные варианты для английского языка: anime, realistic, cyberpunk, meha.\n\n" +
		"У бота есть inline режим! Можно написать @pukechbot и промпт для генерации изображения. В inline режиме нет нужды добавлять его в групповой чат!"
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatId,
		Text:            msgText,
		ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
	})
	utils.ProcessSendMessageError(err, chatId)
}

func processHelp(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := update.Message.Chat.ID
	msgText := "Привет странник! У тебя есть превосходная возможность разнообразить серые будни своих групповых чатов безудержным весельем ежедневных розыгрышей!\n\nЕсли добавить этого бота в чат, то ежеденевно в 12:00 UTC он начнёт проводить розыгрыши среди активных участников чата! В конкурсе участвуют те пользователи, которые отправили хотя бы одно сообщение в течение 12 часов до розыгрыша!\n\n" +
		"Приз можно изменить! Это может сделать суперадмин бота, либо его админ! Суперадмином устанавливаются админы чата и его владелец! Суперадмины могут управлять админами с помощью команд /set_admin, /unset_admin. Для изменения приза достаточно написать\"Сегодня развесёлое нихуя\" или \"Завтра волшебное нихуя\"! И вуаля! Приз на указанный день изменён!\n\nПоздравляю! Теперь ваш чат превратился в оплот ежедневного ураганного веселья!"
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatId,
		Text:            msgText,
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
