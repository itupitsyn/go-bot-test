package bot

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestGetTranscribableFileID(t *testing.T) {
	cases := []struct {
		name    string
		message *models.Message
		want    string
	}{
		{"nil", nil, ""},
		{"voice", &models.Message{Voice: &models.Voice{FileID: "voice"}}, "voice"},
		{"audio", &models.Message{Audio: &models.Audio{FileID: "audio"}}, "audio"},
		{"video note", &models.Message{VideoNote: &models.VideoNote{FileID: "note"}}, "note"},
		{"video", &models.Message{Video: &models.Video{FileID: "video"}}, "video"},
		{
			"audio document",
			&models.Message{Document: &models.Document{FileID: "doc", MimeType: "audio/mpeg"}},
			"doc",
		},
		{
			"video document",
			&models.Message{Document: &models.Document{FileID: "doc", MimeType: "video/mp4"}},
			"doc",
		},
		{
			"pdf document is not worth sending",
			&models.Message{Document: &models.Document{FileID: "doc", MimeType: "application/pdf"}},
			"",
		},
		{"plain text", &models.Message{Text: "расшифруй"}, ""},
	}

	for _, c := range cases {
		if got := getTranscribableFileID(c.message); got != c.want {
			t.Errorf("%s: want %q, got %q", c.name, c.want, got)
		}
	}
}
