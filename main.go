package main

import (
	"log"
	"telebot/bot"
	"telebot/database"
	"telebot/model"
	"telebot/raffleLogic"

	"github.com/joho/godotenv"
)

func main() {
	loadEnv()
	loadDatabase()
	go raffleLogic.Listen()
	bot.Listen()
}

func loadEnv() {
	err := godotenv.Load(".env.local")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
}

func loadDatabase() {
	database.Connect()
	database.Database.AutoMigrate(&model.User{})
	database.Database.AutoMigrate(&model.Prize{})
	database.Database.AutoMigrate(&model.Raffle{})
	database.Database.AutoMigrate(&model.Admin{})
}
