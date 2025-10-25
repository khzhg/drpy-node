package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Listen     string      `json:"listen"`
	AllowHosts []string    `json:"allowHosts"`
	CORS       CORSConfig  `json:"cors"`
	Sign       SignConfig  `json:"sign"`
	Limits     LimitsConfig `json:"limits"`
	Rewrite    RewriteConfig `json:"rewrite"`
	Cache      CacheConfig  `json:"cache"`
	Headers    []HeaderRule `json:"headers"`
	Upstream   UpstreamConfig `json:"upstream"`
}

type CORSConfig struct {
	Origins []string `json:"origins"`
}

type SignConfig struct {
	Enabled    bool   `json:"enabled"`
	Secret     string `json:"secret"`
	TTLSeconds int    `json:"ttlSeconds"`
}

type LimitsConfig struct {
	MaxPlaylistKB int `json:"maxPlaylistKB"`
	MaxURLLength  int `json:"maxURLLength"`
}

type RewriteConfig struct {
	EnableM3U8      bool `json:"enableM3U8"`
	KeepAllVariants bool `json:"keepAllVariants"`
}

type CacheConfig struct {
	M3U8 CacheEntry `json:"m3u8"`
	Key  CacheEntry `json:"key"`
	TS   CacheEntry `json:"ts"`
}

type CacheEntry struct {
	Enabled    bool `json:"enabled"`
	TTLSeconds int  `json:"ttlSeconds"`
	MaxEntries int  `json:"maxEntries"`
}

type HeaderRule struct {
	Match       string            `json:"match"`
	UseRegex    bool              `json:"useRegex"`
	Set         map[string]string `json:"set"`
	HostRewrite bool              `json:"hostRewrite"`
}

type UpstreamConfig struct {
	TimeoutMs       int    `json:"timeoutMs"`
	FollowRedirects bool   `json:"followRedirects"`
	MaxRedirects    int    `json:"maxRedirects"`
	HTTPProxy       string `json:"httpProxy"`
	Socks5          string `json:"socks5"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Set defaults
	if cfg.Sign.TTLSeconds == 0 {
		cfg.Sign.TTLSeconds = 600
	}
	if cfg.Limits.MaxPlaylistKB == 0 {
		cfg.Limits.MaxPlaylistKB = 512
	}
	if cfg.Limits.MaxURLLength == 0 {
		cfg.Limits.MaxURLLength = 2048
	}
	if cfg.Upstream.TimeoutMs == 0 {
		cfg.Upstream.TimeoutMs = 15000
	}
	if cfg.Upstream.MaxRedirects == 0 {
		cfg.Upstream.MaxRedirects = 5
	}

	return &cfg, nil
}