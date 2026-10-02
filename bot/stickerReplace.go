package bot

import (
	"context"
	"fmt"
	"log"
	"telebot/aiApi"
	"telebot/model"
	"telebot/utils"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Why one sticker can be redrawn at all.
//
// The edit model grows an extra limb now and then — a second thumb, a third
// shoulder, an arm out of the wrong side — and it does so at random. Measured
// 02.10.2026 on four seeds of three emotions: the defect appears in roughly
// one frame in four to eight, which is one to three spoiled stickers in a pack
// of ten. Rewording the prompt does not help; the attempt to simplify the
// poses made approve worse, not better.
//
// Nor can the bot find the bad one by itself: telling three shoulders from two
// needs eyes. The person has them and spots it in a second, so the cheapest
// honest answer is to let them point at the sticker and get another take of
// that one.
//
// The whole pack is NOT remade for this. Ten generations to fix one picture is
// four minutes of a card for nothing, and it would roll the dice again on the
// nine that came out fine.

// stickerReplaceNoPackText answers someone who has no pack to mend.
const stickerReplaceNoPackText = "У тебя ещё нет набора. Пришли фото с подписью " +
	"/stickers — сделаю."

// stickerReplaceForeignText answers a sticker from somebody else's set.
const stickerReplaceForeignText = "Этот стикер не из твоего нынешнего набора — " +
	"переделать могу только его. Если набор пересоздавался, возьми стикер из " +
	"нового."

// stickerReplaceNoPhotoText answers a pack made before the photo was kept.
// Such a pack has nothing to redraw from, and the only way out is to make it
// again, which also stores the photo for next time.
const stickerReplaceNoPhotoText = "Этот набор сделан до того, как я научился " +
	"переделывать стикеры по одному, и фотографии от него у меня не осталось. " +
	"Пришли фото с подписью /stickers — обновлю набор целиком, и дальше можно " +
	"будет менять по одному."

// stickerReplaceUnknownText answers a sticker we cannot place. The pack is
// recognised by emoji, so this means the set holds something we did not put
// there.
const stickerReplaceUnknownText = "Не понимаю, какая это эмоция. Переделать могу " +
	"только стикер из набора, который сделал сам."

// stickerReplaceGoneText answers a set that Telegram no longer has.
//
// It happens when the person removed the pack from the Telegram side while our
// row about it stayed. Nothing can be replaced in a set that is not there, and
// the row is dropped along with the answer so that the next /stickers builds a
// new pack instead of trying to refill a ghost.
const stickerReplaceGoneText = "Этого набора у Telegram больше нет. Пришли фото " +
	"с подписью /stickers — сделаю новый."

// stickerReplaceBusyText warns about the wait. One picture is about half a
// minute plus the queue, which is short enough to say so plainly.
const stickerReplaceBusyText = "Перерисовываю этот стикер."

// processStickerReplace draws one emotion again and puts it in place of the
// sticker the person pointed at.
//
// Pointing is done by replying to the sticker with /stickers. There is no
// separate command for it on purpose: /stickers already means "make me
// stickers out of this", and aimed at a single sticker it means exactly this.
func processStickerReplace(ctx context.Context, b *bot.Bot, update *models.Update,
	sticker *models.Sticker) {
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
		log.Println("[error] sticker replace: pack lookup failed", err)
		reply(serverDeadText)
		return
	}
	if pack == nil {
		reply(stickerReplaceNoPackText)
		return
	}

	// Чужой стикер трогать нельзя, и дело не только в вежливости: добавлять в
	// набор Telegram разрешает только его владельцу, так что попытка всё равно
	// кончилась бы отказом, но уже после генерации.
	if sticker.SetName != pack.Name {
		reply(stickerReplaceForeignText)
		return
	}
	if pack.PhotoFileID == "" {
		reply(stickerReplaceNoPhotoText)
		return
	}

	emotion, ok := aiApi.StickerEmotionByEmoji(sticker.Emoji)
	if !ok {
		reply(stickerReplaceUnknownText)
		return
	}

	if replyIfMaintenance(ctx, b, message) {
		return
	}

	// Одна картинка — одна единица лимита. Списываем до работы, как и набор.
	usage, refused := replyIfOverAiLimit(ctx, b, message, aiKindSticker, 1)
	if refused {
		return
	}

	imageBytes, name, err := downloadTelegramFile(ctx, b, pack.PhotoFileID)
	if err != nil {
		// Файл мог исчезнуть вместе с сообщением, из которого брался. Это не
		// поломка бота, и человеку надо сказать, что делать, а не «ошибка».
		log.Println("[error] sticker replace: cannot download the source photo", err)
		if setErr := usage.SetUnits(0); setErr != nil {
			log.Println("[error] sticker replace: cannot refund", setErr)
		}
		reply(stickerReplaceNoPhotoText)
		return
	}

	wait := newChatWaitMessage(ctx, b, chatID, message.ID)
	wait.setText(stickerReplaceBusyText)

	caller := messageCaller(message, wait)
	png, err := aiApi.GenerateSticker(
		aiApi.EditImage{Bytes: imageBytes, Name: name}, emotion, pack.WithBody,
		caller)
	wait.done()
	b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: wait.id()})

	if err != nil {
		log.Printf("[error] sticker replace %s failed: %v\n", emotion.Key, err)
		if setErr := usage.SetUnits(0); setErr != nil {
			log.Println("[error] sticker replace: cannot refund", setErr)
		}
		reply(generationErrorText(err))
		return
	}

	if err := replaceStickerInSet(ctx, b, userID, pack.Name, sticker.FileID,
		emotion, png); err != nil {
		log.Printf("[error] sticker %s not replaced in %s: %v\n",
			emotion.Key, pack.Name, err)

		if isStickerSetMissing(err) {
			if delErr := model.DeleteStickerPack(userID); delErr != nil {
				log.Println("[error] sticker replace: ghost record not deleted", delErr)
			}
			reply(stickerReplaceGoneText)

			return
		}

		reply(serverDeadText)

		return
	}

	reply(fmt.Sprintf("Переделал %s. Набор: %s", sticker.Emoji,
		stickerPackLink(pack.Name)))
}

// replaceStickerInSet swaps one picture for another, keeping its place.
//
// replaceStickerInSet is used rather than add-then-delete because it holds the
// position: adding puts the new sticker at the end, and the pack would slowly
// reorder itself with every mend until the emotions sat in the order they were
// fixed in.
func replaceStickerInSet(ctx context.Context, b *bot.Bot, userID int64,
	name, oldFileID string, emotion aiApi.StickerEmotion, png []byte) error {
	fileID, err := uploadSticker(ctx, b, userID, 0, png)
	if err != nil {
		return fmt.Errorf("uploading the replacement: %w", err)
	}

	if _, err := b.ReplaceStickerInSet(ctx, &bot.ReplaceStickerInSetParams{
		UserID:     userID,
		Name:       name,
		OldSticker: oldFileID,
		Sticker: models.InputSticker{
			Sticker:   &models.InputFileString{Data: fileID},
			Format:    "static",
			EmojiList: emotion.Emoji,
		},
	}); err != nil {
		return fmt.Errorf("replacing in %s: %w", name, err)
	}

	// Обложка набора могла указывать на стикер, которого больше нет. Сброс —
	// косметика, падать из-за него нечего, он сам себя логирует.
	resetStickerSetThumbnail(ctx, b, userID, name)

	return nil
}
