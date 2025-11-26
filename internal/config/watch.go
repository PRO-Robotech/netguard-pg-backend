package config

import (
	"fmt"
	"time"
)

// WatchConfig describes backend watch subsystem settings.
type WatchConfig struct {
	CacheSize        int           `yaml:"cache_size" env:"WATCH_CACHE_SIZE"`
	PGChannel        string        `yaml:"pg_channel" env:"WATCH_PG_CHANNEL"`
	PollInterval     time.Duration `yaml:"poll_interval" env:"WATCH_POLL_INTERVAL"`
	BookmarkInterval time.Duration `yaml:"bookmark_interval" env:"WATCH_BOOKMARK_INTERVAL"`
	AllowBookmarks   bool          `yaml:"allow_bookmarks" env:"WATCH_ALLOW_BOOKMARKS"`
}

// DefaultWatchConfig returns production-ready defaults.
func DefaultWatchConfig() WatchConfig {
	return WatchConfig{
		CacheSize:        10_000,
		PGChannel:        "k8s_resource_changes",
		PollInterval:     500 * time.Millisecond,
		BookmarkInterval: 30 * time.Second,
		AllowBookmarks:   true,
	}
}

// Validate checks that the configuration is sane.
func (c WatchConfig) Validate() error {
	if c.CacheSize <= 0 {
		return fmt.Errorf("watch.cache_size must be positive")
	}
	if c.PGChannel == "" {
		return fmt.Errorf("watch.pg_channel is required")
	}
	if c.PollInterval <= 0 {
		return fmt.Errorf("watch.poll_interval must be positive")
	}
	if c.BookmarkInterval <= 0 {
		return fmt.Errorf("watch.bookmark_interval must be positive")
	}
	return nil
}
