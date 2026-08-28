package bot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// getBiggestPhoto returns the largest of the sizes Telegram offers for a photo,
// which is the one worth feeding to the generator.
func getBiggestPhoto(photos []models.PhotoSize) *models.PhotoSize {
	if len(photos) == 0 {
		return nil
	}

	biggest := &photos[0]
	for i := range photos {
		if biggest.FileSize < photos[i].FileSize {
			biggest = &photos[i]
		}
	}

	return biggest
}

// getSmallestPhoto returns the smallest of the sizes Telegram offers for a
// photo, which is the one worth serving as a preview.
func getSmallestPhoto(photos []models.PhotoSize) *models.PhotoSize {
	if len(photos) == 0 {
		return nil
	}

	smallest := &photos[0]
	for i := range photos {
		if smallest.FileSize > photos[i].FileSize {
			smallest = &photos[i]
		}
	}

	return smallest
}

// getTranscribableFileID returns the id of the audio or video a message
// carries, or an empty string when it carries neither. Documents count only
// when their type says they are audio or video, so that replying "расшифруй"
// to a random pdf does not travel to the service just to come back an error.
func getTranscribableFileID(message *models.Message) string {
	if message == nil {
		return ""
	}

	switch {
	case message.Voice != nil:
		return message.Voice.FileID
	case message.Audio != nil:
		return message.Audio.FileID
	case message.VideoNote != nil:
		return message.VideoNote.FileID
	case message.Video != nil:
		return message.Video.FileID
	case message.Document != nil:
		mime := message.Document.MimeType
		if strings.HasPrefix(mime, "audio/") || strings.HasPrefix(mime, "video/") {
			return message.Document.FileID
		}
	}

	return ""
}

// downloadTelegramFile pulls the bytes of a file behind its id and returns them
// along with the file name, which the generator wants for the multipart upload.
func downloadTelegramFile(ctx context.Context, b *bot.Bot, fileID string) ([]byte, string, error) {
	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, "", fmt.Errorf("getting file %s: %w", fileID, err)
	}

	res, err := http.Get(b.FileDownloadLink(file))
	if err != nil {
		return nil, "", fmt.Errorf("downloading file %s: %w", fileID, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("downloading file %s failed with status %d", fileID, res.StatusCode)
	}

	fileBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, "", fmt.Errorf("reading file %s: %w", fileID, err)
	}

	return fileBytes, filepath.Base(file.FilePath), nil
}
