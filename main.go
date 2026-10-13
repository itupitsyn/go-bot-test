package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"telebot/bot"
	"telebot/database"
	"telebot/model"

	"telebot/raffleLogic"

	"github.com/joho/godotenv"
)

func main() {
	loadEnv()
	loadDatabase()

	// SIGTERM обязателен: его посылает `docker stop`, и без него процесс
	// умирал мгновенно посреди генерации — вместе с чужой оплатой,
	// которую некому было вернуть.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt,
		syscall.SIGTERM)
	defer cancel()

	b := bot.New(ctx)
	go pruneAiUsage(ctx)
	go raffleLogic.Listen(b)
	go bot.StartThumbnailServer(ctx, b)
	go bot.WatchShutdown(ctx)

	// Сразу, до первого апдейта: предыдущий процесс мог умереть, не отдав
	// досчитанное. Сервис его досчитал и держит, подобрать можно только нам.
	go bot.RecoverDeliveries(ctx, b)

	bot.Start(ctx, b)

	// Start возвращается, НЕ дожидаясь обработчиков: внутри ProcessUpdate
	// библиотека уводит каждый в отдельную горутину. Если выйти здесь, процесс
	// завершится и оборвёт чужую генерацию вместе со списанной оплатой —
	// именно это и случилось на проверке 13.10.2026 в 22:04.
	if left, ok := bot.WaitForHandlers(bot.HandlerGrace()); ok {
		log.Println("Stopped cleanly, nothing left in flight")
	} else {
		log.Printf("Giving up after %s, %d handler(s) still running\n",
			bot.HandlerGrace(), left)
	}
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
	if err := db.AutoMigrate(&model.Size{}); err != nil {
		log.Fatal("Error migrating Size", err)
	}
	if err := db.AutoMigrate(&model.Depth{}); err != nil {
		log.Fatal("Error migrating Depth", err)
	}
	if err := db.AutoMigrate(&model.InlineImage{}); err != nil {
		log.Fatal("Error migrating InlineImage", err)
	}
	if err := db.AutoMigrate(&model.AiMaintenance{}); err != nil {
		log.Fatal("Error migrating AiMaintenance", err)
	}
	if err := db.AutoMigrate(&model.AiCredit{}); err != nil {
		log.Fatal("Error migrating AiCredit", err)
	}

	if err := db.AutoMigrate(&model.AiPurchase{}); err != nil {
		log.Fatal("Error migrating AiPurchase", err)
	}

	if err := db.AutoMigrate(&model.StickerPack{}); err != nil {
		log.Fatal("Error migrating StickerPack", err)
	}
	if err := db.AutoMigrate(&model.AiLimit{}); err != nil {
		log.Fatal("Error migrating AiLimit", err)
	}
	if err := db.AutoMigrate(&model.AiLimitSettings{}); err != nil {
		log.Fatal("Error migrating AiLimitSettings", err)
	}
	if err := db.AutoMigrate(&model.AiUsage{}); err != nil {
		log.Fatal("Error migrating AiUsage", err)
	}
	if err := db.AutoMigrate(&model.PendingDelivery{}); err != nil {
		log.Fatal("Error migrating PendingDelivery", err)
	}
	// Снимаем уникальность с users.name. Она осталась от прежней схемы, а
	// AutoMigrate ограничения не убирает — в модели поле давно объявлено
	// обычным индексом, и база с кодом разошлись.
	//
	// Уникальность тут неверна по сути. name — это username из Telegram: у
	// многих он пустой, и первый же безымянный участник занимал "", после чего
	// второй не сохранялся вовсе и выпадал из розыгрышей. Плюс username
	// меняется и переходит от человека к человеку. Постоянный идентификатор —
	// это ID, он и есть первичный ключ.
	//
	// IF EXISTS: на новой базе ограничения нет, и это нормальный ход событий.
	if err := db.Exec(`ALTER TABLE users DROP CONSTRAINT IF EXISTS users_name_key`).Error; err != nil {
		log.Fatal("Error dropping users_name_key", err)
	}

	log.Println("Successfully migrated all tables")

	model.Init(db)

	if err := model.PopulateRoles(); err != nil {
		log.Fatal("Error populating roles", err)
	}

	backfilled, err := model.BackfillInlineImageTokens()
	if err != nil {
		log.Fatal("Error backfilling inline image tokens", err)
	}
	if backfilled > 0 {
		log.Printf("Gave a preview token to %d inline images\n", backfilled)
	}
}

// pruneAiUsage throws away spent quota older than the longest window. Nothing
// older than a week can affect a decision, and without this the table would
// grow for as long as the bot runs.
//
// Once a day is often enough, and the first sweep happens at startup: a bot
// that is restarted daily would otherwise never get round to it.
func pruneAiUsage(ctx context.Context) {
	for {
		removed, err := model.PruneAiUsage(time.Now())
		if err != nil {
			log.Println("[error] error pruning ai usage", err)
		} else if removed > 0 {
			log.Printf("Forgot %d spent ai quota rows\n", removed)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(24 * time.Hour):
		}
	}
}
