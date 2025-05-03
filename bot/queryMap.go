package bot

import (
	"sync"
	"time"
)

type callbackQueryData struct {
	query string
	date  time.Time
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

func (c *safeQueryMap) setValue(key string, value string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.value[key] = callbackQueryData{
		query: value,
		date:  time.Now(),
	}
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
