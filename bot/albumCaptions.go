package bot

import (
	"sync"
	"time"

	"github.com/go-telegram/bot/models"
)

// An album comes as separate messages, one per picture, and the caption sits
// on just one of them. A reply can point at any picture, so to know the whole
// album the bot remembers the captions and pictures it has seen.
const (
	albumCaptionTTL = 24 * time.Hour
	// maxAlbumImages bounds the pictures kept per album: Telegram allows ten,
	// and reading each one costs an llm call.
	maxAlbumImages = 4
)

type albumKey struct {
	chatID  int64
	groupID string
}

type albumCaption struct {
	text   string
	images []string
	seen   time.Time
}

type albumCaptionStore struct {
	mu     sync.Mutex
	byKey  map[albumKey]albumCaption
	pruned time.Time
}

var albumCaptions = &albumCaptionStore{byKey: map[albumKey]albumCaption{}}

// note remembers the caption and the picture of an album message.
func (s *albumCaptionStore) note(message *models.Message, now time.Time) {
	if message == nil || message.MediaGroupID == "" {
		return
	}
	image := getDescribableImageFileID(message)
	if message.Caption == "" && image == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if now.Sub(s.pruned) > time.Hour {
		for key, item := range s.byKey {
			if now.Sub(item.seen) > albumCaptionTTL {
				delete(s.byKey, key)
			}
		}
		s.pruned = now
	}

	key := albumKey{message.Chat.ID, message.MediaGroupID}
	item := s.byKey[key]
	if message.Caption != "" {
		item.text = message.Caption
	}
	if image != "" && len(item.images) < maxAlbumImages {
		item.images = append(item.images, image)
	}
	item.seen = now
	s.byKey[key] = item
}

// lookup gives the caption of the album the message belongs to.
func (s *albumCaptionStore) lookup(message *models.Message, now time.Time) string {
	item, ok := s.get(message, now)
	if !ok {
		return ""
	}
	return item.text
}

// images gives the pictures of the album the message belongs to, in the order
// they came.
func (s *albumCaptionStore) images(message *models.Message, now time.Time) []string {
	item, ok := s.get(message, now)
	if !ok {
		return nil
	}
	return append([]string(nil), item.images...)
}

func (s *albumCaptionStore) get(message *models.Message, now time.Time) (albumCaption, bool) {
	if message == nil || message.MediaGroupID == "" {
		return albumCaption{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.byKey[albumKey{message.Chat.ID, message.MediaGroupID}]
	if !ok || now.Sub(item.seen) > albumCaptionTTL {
		return albumCaption{}, false
	}
	return item, true
}

// factImages lists the pictures to read for a fact check: the whole album when
// the bot has seen it, otherwise just the picture of the message itself.
func factImages(message *models.Message, now time.Time) []string {
	own := getDescribableImageFileID(message)
	images := albumCaptions.images(message, now)
	if own == "" {
		return images
	}
	for _, id := range images {
		if id == own {
			return images
		}
	}
	// The album was not seen (say, the bot restarted since): the picture
	// replied to is still there.
	images = append([]string{own}, images...)
	if len(images) > maxAlbumImages {
		images = images[:maxAlbumImages]
	}
	return images
}

// hasMedia tells whether the message carries a picture, a video or a file.
func hasMedia(message *models.Message) bool {
	return len(message.Photo) > 0 || message.Video != nil ||
		message.Document != nil || message.Animation != nil
}
