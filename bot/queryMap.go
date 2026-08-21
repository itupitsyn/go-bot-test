package bot

import (
	"sync"
	"time"
)

// callbackQueryKind tells apart what an inline result button asks for.
type callbackQueryKind int

const (
	// callbackQueryImage draws a picture from the query text.
	callbackQueryImage callbackQueryKind = iota
	// callbackQueryTextVideo animates the query text from scratch.
	callbackQueryTextVideo
	// callbackQueryImageVideo animates a picture from the user's buffer.
	callbackQueryImageVideo
)

// isVideo reports whether the generation belongs to the video queue.
func (k callbackQueryKind) isVideo() bool {
	return k == callbackQueryTextVideo || k == callbackQueryImageVideo
}

type callbackQueryData struct {
	kind  callbackQueryKind
	query string
	// fileID and ownerID are set for callbackQueryImageVideo only. The owner is
	// the user the picture belongs to, which is not necessarily the one pressing
	// the button: the inline message can sit in a group where anybody can.
	fileID  string
	ownerID int64
	date    time.Time
}

type safeQueryMap struct {
	value map[string]callbackQueryData
	mutex sync.Mutex
}

func (c *safeQueryMap) getValue(key string) (callbackQueryData, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	res, ok := c.value[key]
	return res, ok
}

func (c *safeQueryMap) setValue(key string, value callbackQueryData) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	value.date = time.Now()
	c.value[key] = value
}

func (c *safeQueryMap) deleteValue(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.value, key)
}

func (c *safeQueryMap) deleteOldValues() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	keys := make([]string, 0, len(c.value))
	for k := range c.value {
		keys = append(keys, k)
	}

	for _, k := range keys {
		diff := time.Since(c.value[k].date)
		if diff.Hours() >= 1 {
			delete(c.value, k)
		}
	}
}
