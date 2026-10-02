package bot

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"telebot/aiApi"
	"telebot/model"
	"telebot/utils"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// stickerTitleFallback names a pack whose owner gave us nothing to name it by.
const stickerTitleFallback = "Emotions"

// stickerTitleLimit is Telegram's ceiling for a set title.
const stickerTitleLimit = 64

// stickerSetTitle is what the person sees above the pack in the sticker panel,
// and what everyone else sees when they tap a sticker from it.
//
// The name of the owner belongs here: a person who adds packs of three friends
// would otherwise get three sets called the same thing, told apart only by the
// pictures.
//
// The word itself is English so that the title reads the same for everyone,
// whatever language they speak — a sticker travels far beyond the chat it was
// made in. The name comes after a colon and is never inflected: "Emotions:
// Вася" works for a name in any script, while a possessive would have to guess
// grammar from a Telegram profile, and profiles hold anything from "Вася" to
// an emoji.
func stickerSetTitle(from *models.User) string {
	if from == nil {
		return stickerTitleFallback
	}

	name := strings.TrimSpace(from.FirstName)
	if name == "" {
		name = strings.TrimSpace(from.Username)
	}
	if name == "" {
		return stickerTitleFallback
	}

	title := "Emotions: " + name
	if len([]rune(title)) > stickerTitleLimit {
		title = string([]rune(title)[:stickerTitleLimit])
	}

	return title
}

// stickerNoPhotoText is the answer to a bare command. The pack is built from a
// face, and there is nothing to guess from.
const stickerNoPhotoText = "Пришли фотографию с лицом — или ответь этой командой " +
	"на уже присланную, и я сделаю из неё набор стикеров.\n\n" +
	"А если ответить этой командой на стикер из своего набора, я перерисую " +
	"его одного."

// stickerBusyText warns about the wait. Ten pictures at around half a minute
// each is minutes, and a person who is not told that decides the bot is dead.
const stickerBusyText = "Делаю набор стикеров. Это займёт несколько минут — " +
	"я буду показывать, сколько готово."

// stickerPackPrefix starts the set's short name. Telegram requires the name to
// end with _by_<bot>, to be unique across the whole platform and to consist of
// latin letters, digits and underscores.
const stickerPackPrefix = "emo"

// stickerPackName builds the set's short name for a person. The user id makes
// it unique without a counter, and the same person always gets the same name —
// that is what lets a regeneration refill the existing set instead of leaving
// a trail of abandoned ones.
func stickerPackName(userID int64) string {
	return fmt.Sprintf("%s%d_by_%s", stickerPackPrefix, userID, botName)
}

// stickerPackNameDated is the spare name, used when the plain one is
// unusable.
//
// Telegram keeps the short name of a DELETED set reserved: creating under it
// answers "name is already occupied", while adding to it answers
// STICKERSET_INVALID, because there is nothing there any more. A person who
// deleted their pack and asked for a new one would be stuck between those two
// refusals forever — seen live on 29.09.2026. The minute of creation makes the
// name unique without a counter we would have to store and would lose along
// with the deleted row.
func stickerPackNameDated(userID int64) string {
	return fmt.Sprintf("%s%d_%s_by_%s", stickerPackPrefix, userID,
		time.Now().UTC().Format("060102_1504"), botName)
}

// stickerPackLink is the address a person opens to add their pack.
func stickerPackLink(name string) string {
	return "https://t.me/addstickers/" + name
}

// processStickerPack builds the standard pack out of one photo.
//
// The picture is taken from the message itself or from the one it replies to,
// the same way the other picture commands do it: people attach the command to
// a photo about as often as they send both at once.
func processStickerPack(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	chatID := message.Chat.ID

	reply := func(text string) {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatID,
			Text:            text,
			ReplyParameters: &models.ReplyParameters{MessageID: message.ID},
		})
		utils.ProcessSendMessageError(err, chatID)
	}

	photos := message.Photo
	if len(photos) == 0 && message.ReplyToMessage != nil {
		photos = message.ReplyToMessage.Photo
	}
	if len(photos) == 0 {
		// Ответ командой на стикер — просьба переделать именно его. Отдельной
		// команды для этого нет нарочно: «/stickers» уже значит «сделай мне
		// стикер», и в ответ на один стикер это ровно он.
		if message.ReplyToMessage != nil && message.ReplyToMessage.Sticker != nil {
			processStickerReplace(ctx, b, update, message.ReplyToMessage.Sticker)
			return
		}

		reply(stickerNoPhotoText)
		return
	}

	biggest := getBiggestPhoto(photos)
	if biggest == nil {
		reply(stickerNoPhotoText)
		return
	}

	// Профилактика проверяется ЗДЕСЬ, а не в разборе команды: на /stickers без
	// фотографии бот отвечает, что нужна фотография, и говорить про ремонт в
	// этом случае незачем. Зато до скачивания файла: во время профилактики
	// картинку тянуть с Telegram уже не за чем.
	if replyIfMaintenance(ctx, b, message) {
		return
	}

	imageBytes, name, err := downloadTelegramFile(ctx, b, biggest.FileID)
	if err != nil {
		log.Println("[error] sticker pack: cannot download the photo", err)
		reply(serverDeadText)
		return
	}

	total := aiApi.StickerPackSize()

	// Набор — это десять генераций подряд, и в лимите он столько и стоит:
	// счётчик должен отражать нагрузку на карты, а не число команд. Списываем
	// ДО работы, иначе за те минуты, что набор считается, можно запустить ещё
	// десять. Что не израсходовалось, вернём ниже.
	usage, refused := replyIfOverAiLimit(ctx, b, message, aiKindSticker, total)
	if refused {
		return
	}

	wait := newChatWaitMessage(ctx, b, chatID, message.ID)
	wait.setText(stickerBusyText)

	caller := messageCaller(message, wait)
	// The pack is long, and the queue position of each separate picture means
	// nothing to the person waiting. What matters is how many are ready, so the
	// wait message shows that instead.
	caller.Progress = nil

	pack, err := aiApi.GenerateStickers(
		aiApi.EditImage{Bytes: imageBytes, Name: name}, caller,
		func(done, all int) {
			wait.setText(fmt.Sprintf("%s\n\nГотово %d из %d.", stickerBusyText, done, all))
		})
	stickers := pack.Stickers
	wait.done()

	// Эмоция могла не выйти, и тогда набор короче заказанного. Платить за то,
	// чего нет, человек не должен — переписываем списание по факту. Ноль
	// стирает его совсем.
	if setErr := usage.SetUnits(len(stickers)); setErr != nil {
		log.Println("[error] sticker pack: cannot correct the charge", setErr)
	}

	if err != nil {
		log.Println("[error] sticker pack failed:", err)
		b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: wait.id()})
		reply(generationErrorText(err))
		return
	}

	link, err := publishStickerPack(ctx, b, message.From, stickers,
		biggest.FileID, pack.WithBody)
	b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: wait.id()})
	if err != nil {
		log.Println("[error] sticker pack not published:", err)
		reply(serverDeadText)
		return
	}

	short := ""
	if len(stickers) < total {
		short = fmt.Sprintf("\n\nПолучилось %d из %d — остальные не вышли.",
			len(stickers), total)
	}
	reply(fmt.Sprintf("Готово! Набор здесь: %s%s", link, short))
}

// uploadSticker hands one picture to Telegram and returns the file id to
// refer to it by.
//
// The upload is a separate call on purpose, and it is the only way that works.
// The library builds multipart out of the TOP-LEVEL fields of a params struct:
// an *InputFileUpload lying directly in a field becomes an attached file,
// while the same thing inside Stickers []InputSticker goes through plain JSON
// and marshals to the string "@name.png". Telegram then reads that as a URL
// and answers "failed to get HTTP URL content" — which is exactly what the
// first live run got. In UploadStickerFileParams the picture IS a top-level
// field, so it uploads properly, and the file id it returns can be put into
// the set.
func uploadSticker(ctx context.Context, b *bot.Bot, userID int64, index int,
	png []byte) (string, error) {
	file, err := b.UploadStickerFile(ctx, &bot.UploadStickerFileParams{
		UserID: userID,
		Sticker: &models.InputFileUpload{
			Filename: fmt.Sprintf("sticker%d.png", index),
			Data:     bytes.NewReader(png),
		},
		StickerFormat: "static",
	})
	if err != nil {
		return "", err
	}

	return file.FileID, nil
}

// publishStickerPack creates the person's set or refills the one they already
// have, and returns the link to it.
//
// Refilling means deleting the old stickers and adding the new ones. It is
// done in that order on purpose: Telegram refuses to delete the last sticker
// of a set, so the new ones go in first and the old ones leave afterwards,
// which also means the set is never empty in between.
func publishStickerPack(ctx context.Context, b *bot.Bot, from *models.User,
	stickers []aiApi.StickerResult, photoFileID string, withBody bool) (string, error) {
	userID := from.ID
	title := stickerSetTitle(from)

	// The pictures go up once and are reused by every attempt below: a file id
	// from uploadStickerFile is good for as many set operations as we like, and
	// re-uploading ten PNGs per retry would be paying twice for nothing.
	inputs := make([]models.InputSticker, 0, len(stickers))
	for i, s := range stickers {
		fileID, err := uploadSticker(ctx, b, userID, i, s.PNG)
		if err != nil {
			log.Printf("[warn] sticker %s not uploaded: %v\n", s.Emotion.Key, err)
			continue
		}

		inputs = append(inputs, models.InputSticker{
			Sticker:   &models.InputFileString{Data: fileID},
			Format:    "static",
			EmojiList: s.Emotion.Emoji,
		})
	}
	if len(inputs) == 0 {
		return "", fmt.Errorf("no stickers uploaded for %d", userID)
	}

	existing, err := model.GetStickerPack(userID)
	if err != nil {
		// Not knowing what the person had is not a reason to refuse: try to
		// create below, and fall through to refilling if the set is there.
		log.Println("[warn] sticker pack lookup failed:", err)
	}

	// A set we already know about is refilled; creating it again would only
	// collide with itself.
	if existing != nil {
		if err := refillStickerSet(ctx, b, userID, existing.Name, inputs); err != nil {
			log.Printf("[warn] refilling %s failed: %v\n", existing.Name, err)
		} else {
			saveStickerPack(userID, existing.Name, len(inputs), photoFileID, withBody)

			return stickerPackLink(existing.Name), nil
		}
	}

	// Two names are tried. The plain one is what a person keeps for good; the
	// dated one exists because Telegram holds on to the short name of a set
	// that has been DELETED. Without it a person who removed their pack and
	// asked for a new one would be stuck forever: creating says the name is
	// taken, and refilling finds nothing to fill.
	for _, name := range []string{stickerPackName(userID), stickerPackNameDated(userID)} {
		ok, err := b.CreateNewStickerSet(ctx, &bot.CreateNewStickerSetParams{
			UserID:   userID,
			Name:     name,
			Title:    title,
			Stickers: inputs,
		})
		if err == nil && ok {
			saveStickerPack(userID, name, len(inputs), photoFileID, withBody)

			return stickerPackLink(name), nil
		}

		// Always said out loud. Swallowing this is what made the first live
		// failure unreadable: the log showed the fallback dying without a word
		// about why the straight path had not worked.
		log.Printf("[warn] creating set %s failed (ok=%v): %v\n", name, ok, err)

		// The name being taken is the only refusal worth another attempt: the
		// set may be ours from a lost database, so try to refill it.
		if !isStickerSetExists(err) {
			continue
		}

		if err := refillStickerSet(ctx, b, userID, name, inputs); err != nil {
			log.Printf("[warn] refilling %s failed: %v\n", name, err)
			continue
		}

		saveStickerPack(userID, name, len(inputs), photoFileID, withBody)

		return stickerPackLink(name), nil
	}

	return "", fmt.Errorf("no way to publish a set for %d", userID)
}

// saveStickerPack writes the pack down. Failing to remember it does not spoil a
// pack that already exists, so it only complains.
func saveStickerPack(userID int64, name string, count int, photoFileID string,
	withBody bool) {
	if err := model.SaveStickerPack(userID, name, count, photoFileID, withBody); err != nil {
		log.Println("[warn] sticker pack not saved:", err)
	}
}

// refillStickerSet puts the new stickers into an existing set and takes the old
// ones out.
//
// New in, old out, in that order: Telegram refuses to delete the last sticker
// of a set, so removing first could strand it with one picture that can never
// be replaced. It also means the set is never empty in between.
func refillStickerSet(ctx context.Context, b *bot.Bot, userID int64, name string,
	inputs []models.InputSticker) error {
	set, err := b.GetStickerSet(ctx, &bot.GetStickerSetParams{Name: name})
	if err != nil {
		return fmt.Errorf("reading set %s: %w", name, err)
	}

	added := 0
	for _, input := range inputs {
		if _, err := b.AddStickerToSet(ctx, &bot.AddStickerToSetParams{
			UserID:  userID,
			Name:    name,
			Sticker: input,
		}); err != nil {
			log.Printf("[warn] sticker not added to %s: %v\n", name, err)
			continue
		}
		added++
	}

	if added == 0 {
		return fmt.Errorf("nothing added to %s", name)
	}

	for _, old := range set.Stickers {
		if _, err := b.DeleteStickerFromSet(ctx, &bot.DeleteStickerFromSetParams{
			Sticker: old.FileID,
		}); err != nil {
			log.Println("[warn] old sticker not deleted:", err)
		}
	}

	resetStickerSetThumbnail(ctx, b, userID, name)

	return nil
}

// resetStickerSetThumbnail drops the set's own thumbnail so that Telegram goes
// back to showing the first sticker.
//
// A refill leaves the preview stale: the set keeps pointing at the picture it
// was given when it was made, and after the old stickers have been deleted
// that picture is gone while the preview still shows it. Passing no thumbnail
// at all is what the API takes as "use the first sticker", and the parameter
// is omitempty precisely so it can be left out.
//
// A failure here is only cosmetic — the stickers are already in place — so it
// is logged and forgotten.
func resetStickerSetThumbnail(ctx context.Context, b *bot.Bot, userID int64, name string) {
	if _, err := b.SetStickerSetThumbnail(ctx, &bot.SetStickerSetThumbnailParams{
		Name:   name,
		UserID: userID,
		Format: "static",
	}); err != nil {
		log.Printf("[warn] thumbnail of %s not reset: %v\n", name, err)
	}
}

// isStickerSetExists tells the "this name is taken" refusal from the rest.
// Telegram has no code for it, only the description, so the text is what we
// look at.
func isStickerSetExists(err error) bool {
	if err == nil {
		return false
	}

	text := strings.ToLower(err.Error())

	return strings.Contains(text, "sticker set name is already occupied") ||
		strings.Contains(text, "sticker set already exists")
}

// stickerNoPackText answers someone who has nothing to delete.
const stickerNoPackText = "У тебя ещё нет набора. Пришли фото с подписью " +
	"/stickers — сделаю."

// processStickerPackDelete removes the person's set from Telegram entirely.
//
// Deleting is the bot's job, not the person's: a set made by a bot cannot be
// edited from the Telegram interface, only hidden from one's own list, and
// hiding leaves it alive and re-addable by the old link.
//
// The record goes away even when Telegram says there is no such set. That
// happens when the set was already deleted elsewhere, and keeping a row that
// points at nothing would only make the next /stickers try to refill a ghost.
func processStickerPackDelete(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	chatID := message.Chat.ID

	reply := func(text string) {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatID,
			Text:            text,
			ReplyParameters: &models.ReplyParameters{MessageID: message.ID},
		})
		utils.ProcessSendMessageError(err, chatID)
	}

	userID := message.From.ID
	pack, err := model.GetStickerPack(userID)
	if err != nil {
		log.Println("[error] sticker pack lookup failed:", err)
		reply(serverDeadText)
		return
	}
	if pack == nil {
		reply(stickerNoPackText)
		return
	}

	_, err = b.DeleteStickerSet(ctx, &bot.DeleteStickerSetParams{Name: pack.Name})
	if err != nil && !isStickerSetMissing(err) {
		log.Println("[error] sticker set not deleted:", err)
		reply(serverDeadText)
		return
	}

	if err := model.DeleteStickerPack(userID); err != nil {
		log.Println("[error] sticker pack record not deleted:", err)
		reply(serverDeadText)
		return
	}

	reply("Набор удалён. Захочешь новый — пришли фото с подписью /stickers.")
}

// isStickerSetMissing recognizes "there is no such set" among other refusals.
// Telegram gives no code for it, only the description, so the text is what we
// look at.
func isStickerSetMissing(err error) bool {
	if err == nil {
		return false
	}

	text := strings.ToLower(err.Error())

	return strings.Contains(text, "stickerset_invalid") ||
		strings.Contains(text, "sticker set not found") ||
		strings.Contains(text, "sticker set is invalid")
}
