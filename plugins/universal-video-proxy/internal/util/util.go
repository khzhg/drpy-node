package util

import (
	"net/url"
	"strings"
)

func IsAllowedHost(targetURL string, allowHosts []string) bool {
	if len(allowHosts) == 0 {
		return false
	}

	parsed, err := url.Parse(targetURL)
	if err != nil {
		return false
	}

	hostname := parsed.Hostname()
	for _, allowed := range allowHosts {
		if hostname == allowed || strings.HasSuffix(hostname, "."+allowed) {
			return true
		}
	}
	return false
}

func SanitizeURL(input string) string {
	// Basic URL sanitization
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	
	// Ensure it starts with http:// or https://
	if !strings.HasPrefix(input, "http://") && !strings.HasPrefix(input, "https://") {
		return ""
	}
	
	return input
}

func GetContentType(url string) string {
	url = strings.ToLower(url)
	if strings.Contains(url, ".m3u8") {
		return "application/vnd.apple.mpegurl"
	}
	if strings.Contains(url, ".ts") {
		return "video/mp2t"
	}
	if strings.Contains(url, ".mp4") {
		return "video/mp4"
	}
	if strings.Contains(url, ".key") {
		return "application/octet-stream"
	}
	return "application/octet-stream"
}

func BuildSignParams(sign, ts string) string {
	if sign == "" || ts == "" {
		return ""
	}
	return "sign=" + url.QueryEscape(sign) + "&ts=" + url.QueryEscape(ts)
}