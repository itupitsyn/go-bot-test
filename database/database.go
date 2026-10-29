package database

import (
	"fmt"
	"log"
	"os"

	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

// maskedGormLogger отдаёт gorm стандартный логгер вместо его собственного.
//
// Свой он создаёт через log.New(os.Stdout, ...), то есть пишет мимо маски
// секретов (см. utils/secrets.go) — а в строке подключения, которую gorm
// печатает при неудаче, лежит пароль базы. У стандартного логгера выход уже
// под маской, так что достаточно подменить писателя.
//
// Пороги ниже — ровно те, что у gorm по умолчанию: правка меняет дорогу
// вывода, а не то, что выводится.
func maskedGormLogger() gormLogger.Interface {
	return gormLogger.New(log.Default(), gormLogger.Config{
		SlowThreshold: 200 * time.Millisecond,
		LogLevel:      gormLogger.Warn,
		Colorful:      true,
	})
}

func Connect() (*gorm.DB, error) {
	host := os.Getenv("DB_HOST")
	username := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	databaseName := os.Getenv("DB_NAME")
	port := os.Getenv("DB_PORT")

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=UTC", host, username, password, databaseName, port)
	dialector := postgres.Open(dsn)

	return gorm.Open(dialector, &gorm.Config{Logger: maskedGormLogger()})
}

func ConnectToMockDB() (*gorm.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		log.Fatalf("An error '%s' was not expected when opening a stub database connection", err)
	}

	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: db,
	}), &gorm.Config{})

	if err != nil {
		log.Fatalf("An error '%s' was not expected when opening gorm database", err)
	}

	return gormDB, mock
}
