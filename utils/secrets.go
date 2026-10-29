package utils

import (
	"io"
	"log"
	"os"
	"regexp"
	"strings"
)

// Секреты в журнале.
//
// Токен бота лежал в journalctl открытым, и не по чьей-то небрежности: его
// печатает библиотека. Каждый неудачный вызов она оборачивает в ошибку вида
//
//	error do request for method sendMessage, Post "https://.../bot<ТОКЕН>/sendMessage": ...
//
// то есть ПОЛНЫЙ URL, а в URL Telegram токен и есть путь. Дальше эту ошибку
// печатаем мы сами — в двух десятках мест через log.Println(err). Вычистить
// места бесполезно: любая новая строка с ошибкой вернёт утечку, и заметит её
// снова случайный человек, читающий чужой журнал.
//
// Поэтому режем на выходе, в одной точке: log.SetOutput. Мимо неё не пройдёт
// ни наша строка, ни библиотечная (обработчик ошибок go-telegram/bot печатает
// через тот же log). Проверено на всём коде: fmt.Print, прямой os.Stderr и
// свои log.New в репозитории не встречаются.
//
// ПАНИКА ЭТУ МАСКУ НЕ ВИДИТ — здесь раньше было написано обратное, и это
// неправда: сообщение и стек печатает рантайм прямо в stderr, мимо
// log.SetOutput. Проверено дорогой ценой 29.10.2026: `panic(err)` на старте
// бота уложил в журнал VPS полный токен 1292 раза, пока рядом обычные строки
// шли с маской.
//
// Закрыто с двух сторон. Своих `panic(err)` в коде больше нет — фатальное
// идёт через log.Fatalf (bot.New, raffleLogic.runRaffle), а он печатает
// логгером. На случай непредвиденной паники каждая горутина обёрнута в
// utils.ExitOnPanic (panics.go): он ловит её, печатает через маску и
// завершает процесс так же, как завершила бы она. Запускать горутины нужно
// через utils.Go — голый `go fn()` эту защиту обходит.
//
// Внутренний логгер gorm писал в os.Stdout своим log.New, мимо нас, а в
// строке подключения лежит пароль базы. 29.10.2026 ему подменили писателя на
// стандартный логгер — см. maskedGormLogger в database/database.go.
//
// Чем это плохо: журнал перестаёт быть дословным, и при разборе URL из лога
// скопировать в curl уже нельзя. Это дешевле утечки — токен даёт полный доступ
// к боту, вплоть до смены вебхука на чужой сервер.

// botTokenInURL ловит токен прямо в пути Telegram. Отдельно от переменных
// окружения нарочно: даже если переменную переименуют или токен приедет другим
// путём, форма URL останется та же.
//
// Точка с запятой и срез до конца пути: `[A-Za-z0-9_-]+` намеренно не включает
// `/`, иначе маска съела бы и имя метода, по которому только и понятно, что
// именно не получилось.
var botTokenInURL = regexp.MustCompile(`/bot[0-9]+:[A-Za-z0-9_-]+`)

// secretEnv — переменные, значение которых не должно попадать в журнал ни в
// каком виде. Пароль базы здесь потому, что gorm печатает строку подключения
// при неудачном соединении.
var secretEnv = []string{"TELEGRAM_APITOKEN", "AI_API_TOKEN", "DB_PASSWORD"}

const secretMask = "<вырезано>"

// MaskSecrets returns the line with every known secret replaced by a mask.
func MaskSecrets(line string) string {
	line = botTokenInURL.ReplaceAllString(line, "/bot"+secretMask)

	for _, name := range secretEnv {
		value := os.Getenv(name)
		// Пустое и короткое не трогаем, и это не перестраховка: ReplaceAll с
		// пустой строкой вставляет маску между КАЖДОЙ парой символов, то есть
		// незаданная переменная превратила бы журнал в кашу. Короткое значение
		// (вроде "1" в тестовом окружении) вырезало бы все единицы подряд.
		if len(value) < 8 {
			continue
		}
		line = strings.ReplaceAll(line, value, secretMask)
	}

	return line
}

// maskingWriter вычищает секреты из каждой строки журнала перед записью.
type maskingWriter struct {
	to io.Writer
}

func (w maskingWriter) Write(p []byte) (int, error) {
	_, err := w.to.Write([]byte(MaskSecrets(string(p))))
	if err != nil {
		return 0, err
	}

	// Возвращаем длину ТОГО, ЧТО ДАЛИ, а не того, что записали: после вырезания
	// байт меньше, а io.Writer обязан вернуть n < len(p) только вместе с
	// ошибкой. Иначе log счёл бы удачную запись неполной.
	return len(p), nil
}

// HideSecretsInLogs routes the standard logger through the mask. Call it as
// early as possible: everything logged before goes out as is.
func HideSecretsInLogs() {
	log.SetOutput(maskingWriter{to: log.Writer()})
}
