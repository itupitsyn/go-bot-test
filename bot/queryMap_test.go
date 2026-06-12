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
	m.setValue("key", "hello")

	got, ok := m.getValue("key")
	if !ok {
		t.Fatal("want existing key, got missing")
	}
	if got.query != "hello" {
		t.Errorf("want query %q, got %q", "hello", got.query)
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
	m.setValue("key", "hello")
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
