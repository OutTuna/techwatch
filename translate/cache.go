package translate

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/OutTuna/techwatch/config"
)

const cacheTTL = 7 * 24 * time.Hour

type entry struct {
	Text string    `json:"t"`
	At   time.Time `json:"at"`
}

type Cache struct {
	mu   sync.Mutex
	path string
	max  int
	m    map[string]entry
}

// OpenCache loads the cache file; a missing or broken file means an empty cache.
func OpenCache(path string, max int) *Cache {
	c := &Cache{path: path, max: max, m: map[string]entry{}}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &c.m)
	}
	return c
}

func key(lang, s string) string {
	h := sha1.Sum([]byte(lang + "\x00" + s))
	return hex.EncodeToString(h[:])
}

func (c *Cache) Get(lang, s string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key(lang, s)]
	if !ok || time.Since(e.At) > cacheTTL {
		return "", false
	}
	return e.Text, true
}

func (c *Cache) Put(lang, s, text string) {
	c.mu.Lock()
	c.m[key(lang, s)] = entry{text, time.Now()}
	c.mu.Unlock()
}

// Save drops expired entries, keeps the newest max, writes atomically.
func (c *Cache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]string, 0, len(c.m))
	for k, e := range c.m {
		if time.Since(e.At) > cacheTTL {
			delete(c.m, k)
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) > c.max {
		sort.Slice(keys, func(i, j int) bool { return c.m[keys[i]].At.After(c.m[keys[j]].At) })
		for _, k := range keys[c.max:] {
			delete(c.m, k)
		}
	}
	data, err := json.Marshal(c.m)
	if err != nil {
		return err
	}
	return config.WriteFile(c.path, data)
}
