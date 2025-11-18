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
	usr.Save()

	chat := model.Chat{
		ID:   update.Message.Chat.ID,
		Name: update.Message.Chat.Title,
	}
	_, err := chat.Save()
	if err != nil {
		log.Fatal("error saving chat", err)
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
		chatId := update.Message.Chat.ID
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
	from := update.Message.From
	var fromName string
	if from.Username != "" {
		fromName = fmt.Sprintf("@%s", from.Username)
	} else {
		photo.ParseMode = "HTML"
		fromName = fmt.Sprintf("<a href=\"tg://user?id=%d\">%s</a>", from.ID, utils.GetAlternativeName(from))
	}
	photo.Caption = fmt.Sprintf("%s\n%s", fromName, update.Message.Text)

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

func processPrize(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatId := update.Message.Chat.ID
	user := model.User{
		ID: update.Message.From.ID,
	}

	phraseParts := strings.Split(strings.ToLower(update.Message.Text), " ")
	if len(phraseParts) < 2 {
		return
	}

	if !user.CanCreatePrize(chatId) {
		phraze := raffleLogic.GetRandomPhrazeByKey(raffleLogic.WrongAdminKey)
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatId,
			Text:   phraze,
		})
		utils.ProcessSendMessageError(err, chatId)

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

	phraze := raffleLogic.GetRandomPhrazeByKey(raffleLogic.AcceptPrizeKey)

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatId,
		Text:            phraze,
		ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
	})
	utils.ProcessSendMessageError(err, chatId)
}

func processPrizeInfo(ctx context.Context, b *bot.Bot, chatId int64) {
	year, month, day := time.Now().In(time.UTC).Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)

	dates := []datatypes.Date{datatypes.Date(today), datatypes.Date(today.AddDate(0, 0, 1))}
	prizes, _ := model.GetPrizesByDate(dates, chatId)

	prizeToday := raffleLogic.GetPrizeName(nil)
	prizeTomorrow := raffleLogic.GetPrizeName(nil)
	for _, prize := range *prizes {
		if prize.Date == dates[0] {
			prizeToday = raffleLogic.GetPrizeName(&prize)
		} else {
			prizeTomorrow = raffleLogic.GetPrizeName(&prize)
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
	msgText := "Ты можешь отправить сообщение, например, \"Нарисуй котика\", и бот сгенерирует изображение котика. Если в конце будут слова аниме, реалистично, киберпанк или меха, то изображение будет в соответствующей стилистике.\n\nМожно писать на английском, например draw a cat meha\nВозможные варианты для английского языка: anime, realistic, cyberpunk, meha"
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatId,
		Text:            msgText,
		ReplyParameters: &models.ReplyParameters{MessageID: update.Message.ID},
	})
	utils.ProcessSendMessageError(err, chatId)
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
		log.Fatal("Error getting chat admins", err)
	}
	userRoles, err := model.GetChatAdmins(chatId)
	if err != nil {
		log.Fatal("Error getting chat roles", err)
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
				log.Fatal("Error deleting roles", err)
			} else {
				log.Println("Superadmin is automatically removed", userRole.UserID, userRole.ChatID)
			}
		}
	}

	// add new admins
	for _, chatMember := range chatAdmins {
		user := getUserFromChatMember(&chatMember)
		if user == nil {
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
