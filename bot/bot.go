package bot

import (
	"fmt"
	"log"
	"os"
	"strings"
	"telebot/aiApi"
	"telebot/database"
	"telebot/model"
	"telebot/raffleLogic"
	"telebot/utils"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/matterbridge/telegram-bot-api/v6"
	"gorm.io/datatypes"
)

func Listen() {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_APITOKEN"))
	if err != nil {
		panic(err)
	}

	// bot.Debug = true

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30

	// Start polling Telegram for updates.
	updates := bot.GetUpdatesChan(updateConfig)

	log.Println("Listening for messages...")
	for update := range updates {
		if update.Message == nil {
			continue
		}
		log.Println("Received message from", update.Message.From.UserName)

		msgTextLower := strings.ToLower(update.Message.Text)

		if strings.HasPrefix(msgTextLower, "нарисуй ") || strings.HasPrefix(msgTextLower, "draw ") {
			log.Println("Image generation requested by", update.Message.From.UserName)
			processImageGeneration(bot, update)
		} else if strings.HasPrefix(msgTextLower, "/ai_help") || strings.HasPrefix(msgTextLower, "/ai_help@"+bot.Self.UserName) {
			log.Println("AI help requested by", update.Message.From.UserName)
			processAIHelp(bot, update)
		}

		msgType := update.Message.Chat.Type
		if msgType != "group" && msgType != "supergroup" {
			continue
		}

		if !raffleLogic.IsNoReturnPoint() {
			log.Println("Running processParticipation")
			processParticipation(update)
		}

		// TODO: All of this should be handled via Commands https://pkg.go.dev/gopkg.in/tucnak/telebot.v2#readme-commands
		if msgTextLower == "/stats" || strings.HasPrefix(msgTextLower, "/stats@"+bot.Self.UserName) {
			log.Println("Stats requested by", update.Message.From.UserName)
			processStats(bot, update, false)
		} else if msgTextLower == "/stats_full" || strings.HasPrefix(msgTextLower, "/stats_full@"+bot.Self.UserName) {
			log.Println("Full stats requested by", update.Message.From.UserName)
			processStats(bot, update, true)
		} else if strings.HasPrefix(msgTextLower, "сегодня ") || strings.HasPrefix(msgTextLower, "завтра ") {
			log.Println("Prize requested by", update.Message.From.UserName)
			processPrize(bot, update)
		} else if msgTextLower == "/prize" || strings.HasPrefix(msgTextLower, "/prize@"+bot.Self.UserName) {
			log.Println("Prize info requested by", update.Message.From.UserName)
			processPrizeInfo(bot, update.Message.Chat.ID)
		} else if strings.HasPrefix(msgTextLower, "/set_admin") || strings.HasPrefix(msgTextLower, "/set_admin@"+bot.Self.UserName) {
			log.Println("Setting admin requested by", update.Message.From.UserName)
			processSetAdmin(bot, update)
		} else if strings.HasPrefix(msgTextLower, "/unset_admin") || strings.HasPrefix(msgTextLower, "/unset_admin@"+bot.Self.UserName) {
			log.Println("Unsetting admin requested by", update.Message.From.UserName)
			processUnsetAdmin(bot, update)
		} else if strings.HasPrefix(msgTextLower, "/admins") || strings.HasPrefix(msgTextLower, "/admins@"+bot.Self.UserName) {
			log.Println("Admins requested by", update.Message.From.UserName)
			processAdmins(bot, update)
		}
	}
	log.Println("Bot stopped listening")
}

func processStats(bot *tgbotapi.BotAPI, update tgbotapi.Update, full bool) {
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

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)
	msg.ParseMode = "HTML"
	_, err := bot.Send(msg)
	utils.ProcessSendMessageError(err, update.Message.Chat.ID)
}

func processParticipation(update tgbotapi.Update) {
	from := update.Message.From
	name := from.UserName
	log.Println("Participation requested by", name)

	usr := model.User{
		ID:              from.ID,
		Name:            name,
		AlternativeName: utils.GetAlternativeName(from),
	}
	usr.Save()

	raffle := model.Raffle{
		ChatID:       update.Message.Chat.ID,
		Date:         datatypes.Date(time.Now()),
		Participants: []model.User{},
	}
	raffle.Save()

	participants := model.Raffle{
		ChatID: update.Message.Chat.ID,
		Date:   datatypes.Date(time.Now()),
		Name:   update.Message.Chat.Title,
		Participants: []model.User{
			usr,
		},
	}
	participants.Save()
}

func processImageGeneration(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	go aiApi.GetImage(bot, update)
}

func processPrize(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
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
		msg := tgbotapi.NewMessage(chatId, phraze)
		msg.ReplyToMessageID = update.Message.MessageID
		_, err := bot.Send(msg)
		utils.ProcessSendMessageError(err, chatId)

		return
	}

	var date datatypes.Date
	if phraseParts[0] == "сегодня" {
		if raffleLogic.IsNoReturnPoint() {
			msg := tgbotapi.NewMessage(chatId, "ПОЗДНО!")
			msg.ReplyToMessageID = update.Message.MessageID
			_, err := bot.Send(msg)
			utils.ProcessSendMessageError(err, update.Message.Chat.ID)
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

	msg := tgbotapi.NewMessage(chatId, phraze)
	msg.ReplyToMessageID = update.Message.MessageID
	_, err := bot.Send(msg)
	utils.ProcessSendMessageError(err, chatId)
}

func processPrizeInfo(bot *tgbotapi.BotAPI, chatId int64) {
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

	msg := tgbotapi.NewMessage(chatId, fmt.Sprintf("Сегодня — %s \nЗавтра — %s", prizeToday, prizeTomorrow))
	_, err := bot.Send(msg)
	utils.ProcessSendMessageError(err, chatId)
}

func checkSetCommandInitiator(bot *tgbotapi.BotAPI, update tgbotapi.Update) error {
	initiatorUserId := update.Message.From.ID
	chatId := update.Message.Chat.ID
	superAdminChatUserRole := model.ChatUserRole{}
	if result := database.Database.Where(
		"chat_id = ? AND user_id = ? AND role_id = ?", chatId, initiatorUserId, model.SuperAdminRoleID,
	).First(&superAdminChatUserRole); result.Error != nil {
		_, err := bot.Send(tgbotapi.NewMessage(chatId, "Тебе нельзя так делать!"))
		utils.ProcessSendMessageError(err, chatId)
		return result.Error
	}
	return nil
}

func getSetCommandUserID(bot *tgbotapi.BotAPI, update tgbotapi.Update) (int64, error) {
	chatId := update.Message.Chat.ID
	command := update.Message.Text
	parts := strings.Split(command, " ")
	if len(parts) < 2 {
		_, err := bot.Send(tgbotapi.NewMessage(chatId, "Неверная команда"))
		utils.ProcessSendMessageError(err, chatId)
		return 0, fmt.Errorf("invalid command: %s", command)
	}
	userName := strings.Trim(parts[1], "@ ")
	user := model.User{}
	userResult := database.Database.Model(&model.User{}).Where("name = ?", userName).First(&user)
	if userResult.Error != nil {
		_, err := bot.Send(tgbotapi.NewMessage(chatId, "Такого члена чята нет!"))
		utils.ProcessSendMessageError(err, chatId)
		return 0, userResult.Error
	}
	// TODO: We should also check if mentioned user is actually a member of our chat
	return user.ID, nil
}

func setUserRoleViaCommand(bot *tgbotapi.BotAPI, userId int64, chatId int64, roleID int64) error {
	chatUserRole := &model.ChatUserRole{
		ChatID: chatId,
		UserID: userId,
	}
	chatUserRoleResult := database.Database.First(&chatUserRole)
	if chatUserRoleResult.Error != nil {
		log.Println("No role found, creating new one", chatUserRoleResult.Error)
		chatUserRole := model.ChatUserRole{
			ChatID: chatId,
			UserID: userId,
			RoleID: roleID,
		}
		if _, err := chatUserRole.Save(); err != nil {
			log.Println("Error creating role", err)
			return err
		}
	} else {
		if chatUserRole.RoleID == model.SuperAdminRoleID {
			_, err := bot.Send(tgbotapi.NewMessage(chatId, "Ты гэта не трогай його!"))
			utils.ProcessSendMessageError(err, chatId)
			return fmt.Errorf("user %d is super admin", userId)
		}
		chatUserRole.RoleID = roleID
		if _, err := chatUserRole.Save(); err != nil {
			log.Println("Error creating role", err)
			return err
		}
	}
	return nil
}

func processSetAdmin(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	if checkSetCommandInitiator(bot, update) != nil {
		return
	}
	chatId := update.Message.Chat.ID
	userId, err := getSetCommandUserID(bot, update)
	if err != nil {
		return
	}
	err = setUserRoleViaCommand(bot, userId, chatId, model.PrizeCreatorRoleID)
	if err != nil {
		return
	}
	_, err = bot.Send(tgbotapi.NewMessage(chatId, "Одминка выдана!"))
	utils.ProcessSendMessageError(err, chatId)
}

func processUnsetAdmin(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	if checkSetCommandInitiator(bot, update) != nil {
		return
	}
	chatId := update.Message.Chat.ID
	userId, err := getSetCommandUserID(bot, update)
	if err != nil {
		return
	}
	err = setUserRoleViaCommand(bot, userId, chatId, model.PlayerRoleID)
	if err != nil {
		return
	}
	_, err = bot.Send(tgbotapi.NewMessage(chatId, "Одминка отобрана!"))
	utils.ProcessSendMessageError(err, chatId)
}

func processAdmins(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	chatId := update.Message.Chat.ID

	// TODO: Proper solution is left join, but it's time consuming to implement

	var chatUserRoles []model.ChatUserRole
	chatUserRoleResult := database.Database.Where(
		"chat_id = ? AND role_id IN (?, ?)", chatId, model.SuperAdminRoleID, model.PrizeCreatorRoleID,
	).Order("role_id asc").Find(&chatUserRoles)
	if chatUserRoleResult.Error != nil || len(chatUserRoles) == 0 {
		_, err := bot.Send(tgbotapi.NewMessage(chatId, "Админов нет!"))
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
	var users []model.User
	database.Database.Find(&users, user_ids)

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
	msg := tgbotapi.NewMessage(chatId, msgText)
	msg.ParseMode = "HTML"
	_, err := bot.Send(msg)
	utils.ProcessSendMessageError(err, chatId)
}

func processAIHelp(bot *tgbotapi.BotAPI, update tgbotapi.Update) {
	chatId := update.Message.Chat.ID

	msg := tgbotapi.NewMessage(chatId, "[draw | нарисуй] [prompt] [optional: anime | realistic | cyberpunk | meha | аниме | реалистично | киберпанк | меха]")
	msg.ReplyToMessageID = update.Message.MessageID
	_, err := bot.Send(msg)
	utils.ProcessSendMessageError(err, chatId)
}
