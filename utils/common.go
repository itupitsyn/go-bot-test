package utils

import "fmt"

func ProcessSendMessageError(err error, chatId int64) {
	if err != nil {
		fmt.Printf("[error] couldn't send message to %d\n", chatId)
		fmt.Println(err)
	}
}
