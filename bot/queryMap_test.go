package bot

import (
	"testing"
	"time"
)

func newQueryMap() safeQueryMap {
	return safeQueryMap{value: make(map[string]callbackQueryData)}
}

func TestSafeQueryMapSetGet(t *testing.T) {
	m := newQueryMap()
	m.setValue("key", callbackQueryData{query: "hello"})

	got, ok := m.getValue("key")
	if !ok {
		t.Fatal("want existing key, got missing")
	}
	if got.query != "hello" {
		t.Errorf("want query %q, got %q", "hello", got.query)
	}
	if got.kind != callbackQueryImage {
		t.Errorf("want kind %v, got %v", callbackQueryImage, got.kind)
	}
	if got.date.IsZero() {
		t.Error("want date to be stamped on set")
	}
}

func TestSafeQueryMapSetGetVideo(t *testing.T) {
	m := newQueryMap()
	m.setValue("key", callbackQueryData{
		kind:    callbackQueryImageVideo,
		query:   "hello",
		fileID:  "file",
		ownerID: 42,
	})

	got, ok := m.getValue("key")
	if !ok {
		t.Fatal("want existing key, got missing")
	}
	if got.kind != callbackQueryImageVideo {
		t.Errorf("want kind %v, got %v", callbackQueryImageVideo, got.kind)
	}
	if got.fileID != "file" {
		t.Errorf("want fileID %q, got %q", "file", got.fileID)
	}
	if got.ownerID != 42 {
		t.Errorf("want ownerID %d, got %d", 42, got.ownerID)
	}
}

// Both video kinds belong to the video queue: getting this wrong sends the
// generation to the worker that cannot do it.
func TestCallbackQueryKindIsVideo(t *testing.T) {
	tests := []struct {
		name    string
		kind    callbackQueryKind
		isVideo bool
	}{
		{"draw", callbackQueryImage, false},
		{"t2v", callbackQueryTextVideo, true},
		{"i2v", callbackQueryImageVideo, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.kind.isVideo(); got != test.isVideo {
				t.Errorf("want isVideo %v, got %v", test.isVideo, got)
			}
		})
	}
}

func TestSafeQueryMapGetMissing(t *testing.T) {
	m := newQueryMap()

	if _, ok := m.getValue("missing"); ok {
		t.Error("want missing key to report ok=false")
	}
}

func TestSafeQueryMapDeleteValue(t *testing.T) {
	m := newQueryMap()
	m.setValue("key", callbackQueryData{query: "hello"})
	m.deleteValue("key")

	if _, ok := m.getValue("key"); ok {
		t.Error("want key to be deleted")
	}
}

func TestSafeQueryMapDeleteOldValues(t *testing.T) {
	m := newQueryMap()
	m.value["old"] = callbackQueryData{query: "old", date: time.Now().Add(-2 * time.Hour)}
	m.value["fresh"] = callbackQueryData{query: "fresh", date: time.Now()}

	m.deleteOldValues()

	if _, ok := m.getValue("old"); ok {
		t.Error("want value older than 1h to be deleted")
	}
	if _, ok := m.getValue("fresh"); !ok {
		t.Error("want recent value to be kept")
	}
}
