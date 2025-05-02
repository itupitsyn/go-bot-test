package utils

import (
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
)

func ProcessSendMessageError(err error, chatId int64) {
	if err != nil {
		fmt.Printf("[error] couldn't send message to %d\n", chatId)
		fmt.Println(err)
	}
}

func GetAlternativeName(user *models.User) string {
	var nameParts []string

	if user.FirstName != "" {
		nameParts = append(nameParts, user.FirstName)
	}
	if user.LastName != "" {
		nameParts = append(nameParts, user.LastName)
	}

	return strings.Join(nameParts, "")
}
