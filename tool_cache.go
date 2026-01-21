package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type cacheEntry struct {
	value     string
	expiresAt time.Time
	createdAt time.Time
}

type ToolCache struct {
	mu         sync.Mutex
	items      map[string]cacheEntry
	ttl        time.Duration
	maxEntries int
}

const (
	defaultToolCacheMaxEntries = 256
)

func NewToolCache(ttl time.Duration, maxEntries int) *ToolCache {
	if maxEntries <= 0 {
		maxEntries = defaultToolCacheMaxEntries
	}
	return &ToolCache{
		items:      make(map[string]cacheEntry),
		ttl:        ttl,
		maxEntries: maxEntries,
	}
}

func (c *ToolCache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.items[key]
	if !ok {
		return "", false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.items, key)
		return "", false
	}
	return entry.value, true
}

func (c *ToolCache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := cacheEntry{
		value:     value,
		createdAt: time.Now(),
		expiresAt: time.Now().Add(c.ttl),
	}
	c.items[key] = entry

	if c.maxEntries > 0 && len(c.items) > c.maxEntries {
		c.evictOldest(len(c.items) - c.maxEntries)
	}
}

func (c *ToolCache) evictOldest(count int) {
	if count <= 0 {
		return
	}
	type kv struct {
		key       string
		createdAt time.Time
	}
	var items []kv
	for k, v := range c.items {
		items = append(items, kv{key: k, createdAt: v.createdAt})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].createdAt.Before(items[j].createdAt)
	})
	for i := 0; i < count && i < len(items); i++ {
		delete(c.items, items[i].key)
	}
}

var (
	toolCache     *ToolCache
	toolCacheOnce sync.Once
)

func getToolCache() *ToolCache {
	toolCacheOnce.Do(func() {
		ttlSeconds := parseEnvInt("TOOL_CACHE_TTL", 0)
		if ttlSeconds <= 0 {
			toolCache = nil
			return
		}
		maxEntries := parseEnvInt("TOOL_CACHE_MAX", defaultToolCacheMaxEntries)
		toolCache = NewToolCache(time.Duration(ttlSeconds)*time.Second, maxEntries)
	})
	return toolCache
}

func parseEnvInt(key string, def int) int {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return n
}

func buildToolCacheKey(toolName string, providerName string, args map[string]interface{}) string {
	sanitized := sanitizeToolCacheArgs(args)
	normalized := canonicalJSON(sanitized)
	payload := fmt.Sprintf("%s|%s|%s", toolName, providerName, normalized)
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", sum)
}

func sanitizeToolCacheArgs(args map[string]interface{}) map[string]interface{} {
	if args == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(args))
	for k, v := range args {
		switch k {
		case "stream", "stream_mode", "stderr_stream", "mcp_logging", "mcp_progress":
			continue
		default:
			out[k] = v
		}
	}
	return out
}

func canonicalJSON(v interface{}) string {
	var buf bytes.Buffer
	writeCanonical(&buf, v)
	return buf.String()
}

func writeCanonical(buf *bytes.Buffer, v interface{}) {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case string:
		data, _ := json.Marshal(t)
		buf.Write(data)
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case int:
		buf.WriteString(strconv.Itoa(t))
	case int64:
		buf.WriteString(strconv.FormatInt(t, 10))
	case float32:
		buf.WriteString(strconv.FormatFloat(float64(t), 'f', -1, 32))
	case float64:
		buf.WriteString(strconv.FormatFloat(t, 'f', -1, 64))
	case []interface{}:
		buf.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeCanonical(buf, item)
		}
		buf.WriteByte(']')
	case map[string]interface{}:
		buf.WriteByte('{')
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			keyBytes, _ := json.Marshal(k)
			buf.Write(keyBytes)
			buf.WriteByte(':')
			writeCanonical(buf, t[k])
		}
		buf.WriteByte('}')
	default:
		buf.WriteString(fmt.Sprintf("%q", fmt.Sprintf("%v", t)))
	}
}
