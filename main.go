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
	if err := database.Connect(); err != nil {
		log.Fatal("Error connecting to the database", err)
	}
	log.Println("Successfully connected to the database")
	log.Println("Migrating all tables...")
	if err := database.Database.AutoMigrate(&model.User{}); err != nil {
		log.Fatal("Error migrating User", err)
	}
	if err := database.Database.AutoMigrate(&model.Prize{}); err != nil {
		log.Fatal("Error migrating Prize", err)
	}
	if err := database.Database.AutoMigrate(&model.Raffle{}); err != nil {
		log.Fatal("Error migrating Raffle", err)
	}
	if err := database.Database.AutoMigrate(&model.Admin{}); err != nil {
		log.Fatal("Error migrating Admin", err)
	}
	if err := database.Database.AutoMigrate(&model.Phraze{}); err != nil {
		log.Fatal("Error migrating Phraze", err)
	}
	log.Println("Successfully migrated all tables")
}
