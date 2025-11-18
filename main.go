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
	if err := db.AutoMigrate(&model.Chat{}); err != nil {
		log.Fatal("Error migrating Chat", err)
	}
	if db.Migrator().HasColumn(&model.Raffle{}, "Name") {
		var raffles []model.Raffle
		result := db.Find(&raffles)

		if result.Error != nil {
			log.Fatal(result.Error)
		}

		for _, val := range raffles {
			chat := model.Chat{
				ID:   val.ChatID,
				Name: val.Name,
			}
			db.Save(&chat)
		}
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

	shouldMigrateRoleData := !db.Migrator().HasColumn(&model.ChatUserRole{}, "IsSetManually")
	if err := db.AutoMigrate(&model.ChatUserRole{}); err != nil {
		log.Fatal("Error migrating ChatUserRole", err)
	}

	if shouldMigrateRoleData {
		var roles []model.ChatUserRole
		result := db.Find(&roles)
		if result.Error != nil {
			log.Fatal(result.Error)
		}
		for _, val := range roles {
			val.IsSetManually = true
			db.Save(val)
		}
	}

	log.Println("Successfully migrated all tables")

	model.Init(db)

	if err := model.PopulateRoles(); err != nil {
		log.Fatal("Error populating roles", err)
	}
}
