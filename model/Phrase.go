package model

import (
	"fmt"

	"gorm.io/gorm"
)

// Phraze — одна строчка, которую бот говорит в чат. Группа собирает строчки в
// сценарий: он выбирается целиком и выдаётся по порядку (GetRandomPhrazeByKey).
//
// ID — суррогатный. До 20.10.2026 первичным ключом была пара (key, value), то
// есть именем строки служил сам её текст, и одна и та же фраза не могла стоять
// в двух группах одного ключа — админка отдавала 500 на сохранении. А общая
// концовка у двух сценариев — обычное дело: «КРУТИМ!!!» в конце и того, и
// другого. Текст — это содержимое строки, а не её имя.
type Phraze struct {
	ID            uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Key           string `gorm:"size:255;not null;index"`
	Value         string `gorm:"not null;"`
	IsUncensored  bool   `json:"is_uncensored"`
	IsWithSpoiler bool   `gorm:"default:false" json:"is_with_spoiler"`
	Group         *int
	Order         *int
}

// GivePhrazesAnID переводит таблицу с первичного ключа (key, value) на
// собственный ID и тем разрешает одной фразе стоять в двух группах.
//
// Вызывать до AutoMigrate: тот первичный ключ у готовой таблицы не меняет, зато
// сам добавит колонку id и оставит старый ключ на месте — пара (key, value) так
// и останется уникальной, а молча.
//
// Повторный вызов ничего не делает: на новой базе таблицы ещё нет, на уже
// переведённой ключ стоит там, где нужно. Три ALTER идут одной транзакцией,
// чтобы оборванная миграция не оставила таблицу вовсе без ключа.
func GivePhrazesAnID(db *gorm.DB) error {
	var pk struct {
		Name    string
		Columns string
	}

	err := db.Raw(`
		SELECT c.conname AS name,
			string_agg(a.attname, ',' ORDER BY a.attnum) AS columns
		FROM pg_constraint c
		JOIN pg_attribute a
			ON a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
		WHERE c.conrelid = to_regclass('phrazes') AND c.contype = 'p'
		GROUP BY c.conname`).Scan(&pk).Error
	if err != nil {
		return err
	}

	if pk.Name == "" || pk.Columns == "id" {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`ALTER TABLE phrazes ADD COLUMN IF NOT EXISTS id bigserial`).Error; err != nil {
			return err
		}

		// Имя ключа читаем из базы, а не пишем руками: у таблицы, созданной не
		// нами, оно может быть любым.
		if err := tx.Exec(fmt.Sprintf(`ALTER TABLE phrazes DROP CONSTRAINT %q`, pk.Name)).Error; err != nil {
			return err
		}

		return tx.Exec(`ALTER TABLE phrazes ADD PRIMARY KEY (id)`).Error
	})
}

// Order() must come BEFORE Find(). Find is a finisher: it runs the query, and
// anything chained after it never reaches the SQL. Written the other way round
// these two silently returned rows in whatever order Postgres felt like, which
// matters because GetRandomPhrazeByKey appends a whole group in the order it
// receives it and does not sort by Order itself.
func getAnyPharzesByKey(key string) (*[]Phraze, error) {
	var result []Phraze
	err := db.Model(&Phraze{}).Where("key = ?", key).Order("\"group\" ASC, \"order\" ASC").Find(&result).Error

	return &result, err
}

// The flag means two different things on the two tables, and they are easy to
// confuse: chats.is_uncensored says the CHAT is allowed rude phrases, while
// phrazes.is_uncensored says the PHRASE itself is rude. This function serves
// the chats where rude phrases are NOT allowed, so it must take the polite
// ones — is_uncensored = false. It asked for true until 13.10.2026, which
// handed censored chats exactly the phrases they are meant to be spared.
func getCensoredPharzesByKey(key string) (*[]Phraze, error) {
	var result []Phraze
	err := db.Model(&Phraze{}).Where("key = ? and is_uncensored = ?", key, false).Order("\"group\" ASC, \"order\" ASC").Find(&result).Error

	return &result, err
}

func GetPharzesByKey(key string, isUncensored bool) (*[]Phraze, error) {
	if isUncensored {
		return getAnyPharzesByKey(key)
	}

	return getCensoredPharzesByKey(key)
}
