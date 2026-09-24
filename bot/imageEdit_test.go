package bot

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func photo(id string, size int) []models.PhotoSize {
	return []models.PhotoSize{{FileID: id, FileSize: size}}
}

// The order here is part of the contract: the service treats several images as
// a mix, and the order decides what goes into what.
func TestEditPhotos(t *testing.T) {
	cases := []struct {
		name    string
		message *models.Message
		want    []string
	}{
		{
			name:    "без картинок",
			message: &models.Message{Text: "нарисуй котика"},
			want:    nil,
		},
		{
			name:    "картинка в самом сообщении",
			message: &models.Message{Caption: "нарисуй ей рыжие волосы", Photo: photo("own", 10)},
			want:    []string{"own"},
		},
		{
			name: "ответ на картинку",
			message: &models.Message{
				Text:           "нарисуй ей рыжие волосы",
				ReplyToMessage: &models.Message{Photo: photo("replied", 10)},
			},
			want: []string{"replied"},
		},
		{
			name: "и своя, и в ответе — сперва та, на которую отвечают",
			message: &models.Message{
				Caption:        "посади её за этот стол",
				Photo:          photo("own", 10),
				ReplyToMessage: &models.Message{Photo: photo("replied", 10)},
			},
			want: []string{"replied", "own"},
		},
		{
			name: "ответ на сообщение без картинки",
			message: &models.Message{
				Text:           "нарисуй котика",
				ReplyToMessage: &models.Message{Text: "просто текст"},
			},
			want: nil,
		},
	}

	for _, current := range cases {
		t.Run(current.name, func(t *testing.T) {
			got := editPhotos(current.message)
			if len(got) != len(current.want) {
				t.Fatalf("editPhotos() дал %d картинок, ожидалось %d", len(got), len(current.want))
			}
			for i, want := range current.want {
				if got[i].FileID != want {
					t.Errorf("картинка %d: %q, ожидалось %q", i, got[i].FileID, want)
				}
			}
		})
	}
}

// Of the several sizes of one Telegram photo we need the largest: editing a
// 90x90 preview makes no sense.
func TestEditPhotosTakesBiggestSize(t *testing.T) {
	message := &models.Message{
		Caption: "нарисуй ей рыжие волосы",
		Photo: []models.PhotoSize{
			{FileID: "small", FileSize: 100},
			{FileID: "big", FileSize: 9000},
			{FileID: "medium", FileSize: 2000},
		},
	}

	got := editPhotos(message)
	if len(got) != 1 {
		t.Fatalf("editPhotos() дал %d картинок, ожидалась одна", len(got))
	}
	if got[0].FileID != "big" {
		t.Errorf("взят %q, ожидался самый крупный размер", got[0].FileID)
	}
}
