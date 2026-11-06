package bot

import (
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

func albumPhoto(id string) []models.PhotoSize {
	return []models.PhotoSize{{FileID: id, FileSize: 10}}
}

func TestAlbumCaptionFoundFromAnotherPicture(t *testing.T) {
	store := &albumCaptionStore{byKey: map[albumKey]albumCaption{}}
	now := time.Now()
	chat := models.Chat{ID: 7}

	store.note(&models.Message{Chat: chat, MediaGroupID: "g1", Caption: "текст альбома", Photo: albumPhoto("a")}, now)
	store.note(&models.Message{Chat: chat, MediaGroupID: "g1", Photo: albumPhoto("b")}, now)

	second := &models.Message{Chat: chat, MediaGroupID: "g1", Photo: albumPhoto("b")}
	if got := store.lookup(second, now); got != "текст альбома" {
		t.Fatalf("lookup = %q", got)
	}
	if got := store.images(second, now); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("images = %v", got)
	}
	if got := store.lookup(&models.Message{Chat: models.Chat{ID: 8}, MediaGroupID: "g1"}, now); got != "" {
		t.Fatalf("other chat leaked: %q", got)
	}
	if got := store.lookup(second, now.Add(albumCaptionTTL+time.Minute)); got != "" {
		t.Fatalf("expired caption returned: %q", got)
	}
	if got := store.lookup(&models.Message{Chat: chat}, now); got != "" {
		t.Fatalf("message outside an album got %q", got)
	}
}

func TestAlbumImagesCapped(t *testing.T) {
	store := &albumCaptionStore{byKey: map[albumKey]albumCaption{}}
	now := time.Now()
	chat := models.Chat{ID: 7}
	for _, id := range []string{"1", "2", "3", "4", "5", "6"} {
		store.note(&models.Message{Chat: chat, MediaGroupID: "g", Photo: albumPhoto(id)}, now)
	}
	if got := store.images(&models.Message{Chat: chat, MediaGroupID: "g"}, now); len(got) != maxAlbumImages {
		t.Fatalf("kept %d pictures, want %d", len(got), maxAlbumImages)
	}
}

func TestAlbumCaptionsPruned(t *testing.T) {
	store := &albumCaptionStore{byKey: map[albumKey]albumCaption{}}
	now := time.Now()
	chat := models.Chat{ID: 7}

	store.note(&models.Message{Chat: chat, MediaGroupID: "old", Caption: "a"}, now)
	store.note(&models.Message{Chat: chat, MediaGroupID: "new", Caption: "b"}, now.Add(albumCaptionTTL+2*time.Hour))
	if len(store.byKey) != 1 {
		t.Fatalf("store holds %d captions, want 1", len(store.byKey))
	}
}

func TestFactImages(t *testing.T) {
	saved := albumCaptions
	defer func() { albumCaptions = saved }()
	albumCaptions = &albumCaptionStore{byKey: map[albumKey]albumCaption{}}
	now := time.Now()
	chat := models.Chat{ID: 7}

	single := &models.Message{Chat: chat, Photo: albumPhoto("s")}
	if got := factImages(single, now); len(got) != 1 || got[0] != "s" {
		t.Fatalf("single picture: %v", got)
	}
	if got := factImages(&models.Message{Chat: chat, Text: "просто текст"}, now); len(got) != 0 {
		t.Fatalf("text message got pictures: %v", got)
	}

	albumCaptions.note(&models.Message{Chat: chat, MediaGroupID: "g", Photo: albumPhoto("a")}, now)
	albumCaptions.note(&models.Message{Chat: chat, MediaGroupID: "g", Photo: albumPhoto("b")}, now)
	if got := factImages(&models.Message{Chat: chat, MediaGroupID: "g", Photo: albumPhoto("b")}, now); len(got) != 2 {
		t.Fatalf("album: %v", got)
	}

	// An album the bot never saw: the picture replied to still counts.
	unseen := &models.Message{Chat: chat, MediaGroupID: "x", Photo: albumPhoto("u")}
	if got := factImages(unseen, now); len(got) != 1 || got[0] != "u" {
		t.Fatalf("unseen album: %v", got)
	}
}

func TestJoinPictureText(t *testing.T) {
	if got := joinPictureText(" подпись ", nil); got != "подпись" {
		t.Fatalf("text only: %q", got)
	}
	if got := joinPictureText("", []string{"с картинки"}); got != factPictureLabel+":\nс картинки" {
		t.Fatalf("one picture: %q", got)
	}
	want := "подпись\n\n" + factPictureLabel + " 1:\nпервая\n\n" + factPictureLabel + " 2:\nвторая"
	if got := joinPictureText("подпись", []string{"первая", "вторая"}); got != want {
		t.Fatalf("two pictures: %q", got)
	}
	if got := joinPictureText("", nil); got != "" {
		t.Fatalf("nothing: %q", got)
	}
}
