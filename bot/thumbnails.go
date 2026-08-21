package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"telebot/model"
	"time"

	"github.com/go-telegram/bot"
)

const (
	// thumbnailPath is the prefix every preview URL is served under.
	thumbnailPath = "/img/"
	// defaultThumbnailPort is used when THUMBNAIL_PORT says nothing.
	defaultThumbnailPort = "8080"
	// thumbnailCacheAge is how long clients may hold on to a preview. The
	// picture behind a token never changes, so this can be generous.
	thumbnailCacheAge = 24 * time.Hour
)

// thumbnailURL is the address an inline result points at for its preview. It
// comes back empty when no public address is configured, and the results then
// go without previews rather than pointing nowhere.
func thumbnailURL(token string) string {
	baseURL := strings.TrimSuffix(os.Getenv("PUBLIC_BASE_URL"), "/")
	if baseURL == "" || token == "" {
		return ""
	}

	return baseURL + thumbnailPath + token
}

// serveThumbnail hands out the picture behind a preview token. Everything that
// fails — an unknown token, a picture already rotated out of the buffer, a file
// id Telegram no longer serves — comes back as a plain 404, so that the endpoint
// tells a prober nothing about which tokens exist.
func serveThumbnail(b *bot.Bot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, thumbnailPath)

		image, err := model.GetInlineImageByToken(token)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		// Pictures saved before previews existed have no separate thumbnail, so
		// the full-size one stands in.
		fileID := image.ThumbFileID
		if fileID == "" {
			fileID = image.FileID
		}

		imageBytes, _, err := downloadTelegramFile(r.Context(), b, fileID)
		if err != nil {
			log.Println("[error] error getting thumbnail bytes")
			log.Println(err)
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(thumbnailCacheAge.Seconds())))
		if _, err := w.Write(imageBytes); err != nil {
			log.Println("[error] error writing thumbnail")
			log.Println(err)
		}
	}
}

// StartThumbnailServer serves the previews shown next to inline results. A
// Telegram client fetches a preview by URL and will not take a file id, so the
// pictures have to be reachable from outside; put a proxy in front of this port
// and point PUBLIC_BASE_URL at it. Blocks until the context is cancelled.
func StartThumbnailServer(ctx context.Context, b *bot.Bot) {
	port := os.Getenv("THUMBNAIL_PORT")
	if port == "" {
		port = defaultThumbnailPort
	}

	if os.Getenv("PUBLIC_BASE_URL") == "" {
		log.Println("PUBLIC_BASE_URL is not set, inline results will go without previews")
	}

	mux := http.NewServeMux()
	mux.HandleFunc(thumbnailPath, serveThumbnail(b))

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Println("[error] error stopping the thumbnail server", err)
		}
	}()

	log.Println("Thumbnail server is listening on", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Println("[error] thumbnail server stopped", err)
	}
}
