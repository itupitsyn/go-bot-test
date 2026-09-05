package bot

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-telegram/bot"
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

func TestIsFileTooBig(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			"отказ по размеру",
			fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: file is too big"),
			true,
		},
		{
			"другой bad request",
			fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: wrong file identifier"),
			false,
		},
		{"поломка сети", errors.New("connection refused"), false},
	}

	for _, c := range cases {
		if got := isFileTooBig(c.err); got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}
