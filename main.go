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
	db, err := database.Connect()

	if err != nil {
		log.Fatal("Error connecting to the database", err)
	}
	log.Println("Successfully connected to the database")

	log.Println("Migrating all tables...")
	if err := db.AutoMigrate(&model.User{}); err != nil {
		log.Fatal("Error migrating User", err)
	}
	if err := db.AutoMigrate(&model.Prize{}); err != nil {
		log.Fatal("Error migrating Prize", err)
	}
	if err := db.AutoMigrate(&model.Raffle{}); err != nil {
		log.Fatal("Error migrating Raffle", err)
	}
	if err := db.AutoMigrate(&model.Admin{}); err != nil {
		log.Fatal("Error migrating Admin", err)
	}
	if err := db.AutoMigrate(&model.Phraze{}); err != nil {
		log.Fatal("Error migrating Phraze", err)
	}
	if err := db.AutoMigrate(&model.Role{}); err != nil {
		log.Fatal("Error migrating Role", err)
	}
	if err := db.AutoMigrate(&model.ChatUserRole{}); err != nil {
		log.Fatal("Error migrating ChatUserRole", err)
	}
	log.Println("Successfully migrated all tables")

	model.Init(db)

	if err := model.PopulateRoles(); err != nil {
		log.Fatal("Error populating roles", err)
	}
}
