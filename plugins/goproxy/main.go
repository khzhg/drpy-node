package main

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Configuration struct
type Config struct {
	Listen       string
	Path         string
	Allow        []string
	UserAgent    string
	Referer      string
	Cookie       string
	RuleHostUA   string
	RuleRegexUA  string
	Rewrite      bool
	SignRequired bool
	SignSecret   string
	CacheTTL     int
	CorsOrigins  string
	LogEnabled   bool
	MaxMB        int
}

// Cache entry structure
type CacheEntry struct {
	Data      []byte
	Headers   http.Header
	ExpiresAt time.Time
}

// Global cache
var cache = make(map[string]*CacheEntry)
var cacheMutex sync.RWMutex

// Configuration instance
var config Config

// Helper function to verify HMAC signature
func verifySignature(data, signature, secret string) bool {
	if secret == "" {
		return false
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(data))
	expectedSignature := hex.EncodeToString(h.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// Helper function to generate MD5 hash
func generateMD5(data string) string {
	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:])
}

// Helper function to check if host is allowed
func isHostAllowed(host string) bool {
	if len(config.Allow) == 0 {
		return true // No restrictions if no allow list
	}
	for _, allowed := range config.Allow {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

// Helper function to get user agent based on rules
func getUserAgent(targetURL string) string {
	if config.RuleRegexUA != "" {
		// Parse rule format: regex|ua
		parts := strings.SplitN(config.RuleRegexUA, "|", 2)
		if len(parts) == 2 {
			if matched, _ := regexp.MatchString(parts[0], targetURL); matched {
				return parts[1]
			}
		}
	}
	
	if config.RuleHostUA != "" {
		// Parse rule format: host|ua
		parts := strings.SplitN(config.RuleHostUA, "|", 2)
		if len(parts) == 2 {
			parsedURL, err := url.Parse(targetURL)
			if err == nil && strings.Contains(parsedURL.Host, parts[0]) {
				return parts[1]
			}
		}
	}
	
	if config.UserAgent != "" {
		return config.UserAgent
	}
	
	return "goproxy/1.0"
}

// Helper function to rewrite m3u8 content
func rewriteM3U8(content []byte, baseURL *url.URL, proxyPath string) []byte {
	if !config.Rewrite {
		return content
	}
	
	lines := strings.Split(string(content), "\n")
	var result []string
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			result = append(result, line)
			continue
		}
		
		// Check if line is a URL
		if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			// Absolute URL - proxy it
			encodedURL := url.QueryEscape(line)
			proxyURL := fmt.Sprintf("%s?url=%s", proxyPath, encodedURL)
			result = append(result, proxyURL)
		} else if !strings.HasPrefix(line, "/") {
			// Relative URL - resolve and proxy it
			resolvedURL := baseURL.ResolveReference(&url.URL{Path: line})
			encodedURL := url.QueryEscape(resolvedURL.String())
			proxyURL := fmt.Sprintf("%s?url=%s", proxyPath, encodedURL)
			result = append(result, proxyURL)
		} else {
			result = append(result, line)
		}
	}
	
	return []byte(strings.Join(result, "\n"))
}

// Helper function to get cache key
func getCacheKey(targetURL string, headers map[string]string) string {
	key := targetURL
	for k, v := range headers {
		key += fmt.Sprintf("|%s:%s", k, v)
	}
	return generateMD5(key)
}

// Helper function to get cached response
func getCachedResponse(key string) (*CacheEntry, bool) {
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()
	
	entry, exists := cache[key]
	if !exists || time.Now().After(entry.ExpiresAt) {
		return nil, false
	}
	return entry, true
}

// Helper function to set cached response
func setCachedResponse(key string, data []byte, headers http.Header) {
	if config.CacheTTL <= 0 {
		return
	}
	
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	
	cache[key] = &CacheEntry{
		Data:      data,
		Headers:   headers,
		ExpiresAt: time.Now().Add(time.Duration(config.CacheTTL) * time.Second),
	}
}

// CORS handler
func setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	if config.CorsOrigins != "" {
		origins := strings.Split(config.CorsOrigins, ",")
		origin := r.Header.Get("Origin")
		
		for _, allowedOrigin := range origins {
			allowedOrigin = strings.TrimSpace(allowedOrigin)
			if allowedOrigin == "*" || allowedOrigin == origin {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				break
			}
		}
		
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Max-Age", "3600")
	}
}

// Main proxy handler
func proxyHandler(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w, r)
	
	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	
	// Get target URL from query parameter
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		http.Error(w, "Missing url parameter", http.StatusBadRequest)
		return
	}
	
	// Parse target URL
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "Invalid URL: "+err.Error(), http.StatusBadRequest)
		return
	}
	
	// Check if host is allowed
	if !isHostAllowed(parsedURL.Host) {
		http.Error(w, "Host not allowed: "+parsedURL.Host, http.StatusForbidden)
		return
	}
	
	// Verify signature if required
	if config.SignRequired {
		signature := r.URL.Query().Get("sign")
		if signature == "" || !verifySignature(targetURL, signature, config.SignSecret) {
			http.Error(w, "Invalid or missing signature", http.StatusUnauthorized)
			return
		}
	}
	
	// Build request headers
	requestHeaders := make(map[string]string)
	
	// Set User-Agent
	userAgent := getUserAgent(targetURL)
	requestHeaders["User-Agent"] = userAgent
	
	// Set Referer if configured
	if config.Referer != "" {
		requestHeaders["Referer"] = config.Referer
	}
	
	// Set Cookie if configured
	if config.Cookie != "" {
		requestHeaders["Cookie"] = config.Cookie
	}
	
	// Copy some headers from original request
	for _, header := range []string{"Range", "Accept", "Accept-Language", "Accept-Encoding"} {
		if value := r.Header.Get(header); value != "" {
			requestHeaders[header] = value
		}
	}
	
	// Check cache
	cacheKey := getCacheKey(targetURL, requestHeaders)
	if cachedEntry, found := getCachedResponse(cacheKey); found {
		if config.LogEnabled {
			log.Printf("Cache hit for %s", targetURL)
		}
		
		// Copy cached headers
		for k, v := range cachedEntry.Headers {
			w.Header()[k] = v
		}
		
		w.WriteHeader(http.StatusOK)
		w.Write(cachedEntry.Data)
		return
	}
	
	// Create HTTP client and request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	
	req, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "Failed to create request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	
	// Set headers
	for k, v := range requestHeaders {
		req.Header.Set(k, v)
	}
	
	if config.LogEnabled {
		log.Printf("Proxying %s %s", r.Method, targetURL)
	}
	
	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response: "+err.Error(), http.StatusInternalServerError)
		return
	}
	
	// Check size limit
	if config.MaxMB > 0 && len(body) > config.MaxMB*1024*1024 {
		http.Error(w, "Response too large", http.StatusRequestEntityTooLarge)
		return
	}
	
	// Process m3u8 content if needed
	contentType := resp.Header.Get("Content-Type")
	if config.Rewrite && (strings.Contains(contentType, "application/vnd.apple.mpegurl") || 
		strings.Contains(contentType, "application/x-mpegURL") || 
		strings.HasSuffix(parsedURL.Path, ".m3u8")) {
		
		proxyPath := fmt.Sprintf("http://%s%s", r.Host, config.Path)
		body = rewriteM3U8(body, parsedURL, proxyPath)
	}
	
	// Copy response headers
	responseHeaders := make(http.Header)
	for k, v := range resp.Header {
		// Skip problematic headers
		if k == "Content-Length" || k == "Transfer-Encoding" || k == "Connection" {
			continue
		}
		responseHeaders[k] = v
		w.Header()[k] = v
	}
	
	// Set content length
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	
	// Cache the response
	setCachedResponse(cacheKey, body, responseHeaders)
	
	// Write response
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
	
	if config.LogEnabled {
		log.Printf("Proxied %s %s -> %d (%d bytes)", r.Method, targetURL, resp.StatusCode, len(body))
	}
}

// Health check handler
func healthHandler(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w, r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"goproxy"}`))
}

func main() {
	// Parse command line flags
	flag.StringVar(&config.Listen, "listen", ":8080", "Listen address and port")
	flag.StringVar(&config.Path, "path", "/proxy", "Proxy endpoint path")
	
	// Allow multiple -allow flags
	var allowHosts arrayFlags
	flag.Var(&allowHosts, "allow", "Allowed host (can be repeated)")
	
	flag.StringVar(&config.UserAgent, "ua", "", "User-Agent header")
	flag.StringVar(&config.Referer, "referer", "", "Referer header")
	flag.StringVar(&config.Cookie, "cookie", "", "Cookie header")
	flag.StringVar(&config.RuleHostUA, "rule-host-ua", "", "Host-based UA rule (format: host|ua)")
	flag.StringVar(&config.RuleRegexUA, "rule-regex-ua", "", "Regex-based UA rule (format: regex|ua)")
	flag.BoolVar(&config.Rewrite, "rewrite", false, "Enable m3u8 segment rewriting")
	flag.BoolVar(&config.SignRequired, "sign-required", false, "Require HMAC signature validation")
	flag.StringVar(&config.SignSecret, "sign-secret", "", "HMAC signature secret")
	flag.IntVar(&config.CacheTTL, "cache-ttl", 0, "Cache TTL in seconds (0 = no cache)")
	flag.StringVar(&config.CorsOrigins, "cors-origins", "", "CORS allowed origins (comma-separated)")
	flag.BoolVar(&config.LogEnabled, "log", false, "Enable request logging")
	flag.IntVar(&config.MaxMB, "max-mb", 100, "Maximum response size in MB")
	
	flag.Parse()
	
	// Convert allowHosts to config.Allow
	config.Allow = []string(allowHosts)
	
	// Setup logging
	if config.LogEnabled {
		log.SetOutput(os.Stdout)
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	} else {
		log.SetOutput(io.Discard)
	}
	
	// Setup HTTP server
	http.HandleFunc(config.Path, proxyHandler)
	http.HandleFunc("/health", healthHandler)
	
	// Start cache cleanup goroutine
	if config.CacheTTL > 0 {
		go func() {
			ticker := time.NewTicker(time.Duration(config.CacheTTL) * time.Second)
			defer ticker.Stop()
			
			for range ticker.C {
				cacheMutex.Lock()
				now := time.Now()
				for key, entry := range cache {
					if now.After(entry.ExpiresAt) {
						delete(cache, key)
					}
				}
				cacheMutex.Unlock()
			}
		}()
	}
	
	fmt.Printf("GoProxy server starting on %s\n", config.Listen)
	fmt.Printf("Proxy endpoint: %s\n", config.Path)
	if len(config.Allow) > 0 {
		fmt.Printf("Allowed hosts: %v\n", config.Allow)
	}
	if config.SignRequired {
		fmt.Printf("Signature validation: enabled\n")
	}
	if config.CacheTTL > 0 {
		fmt.Printf("Cache TTL: %d seconds\n", config.CacheTTL)
	}
	
	log.Fatal(http.ListenAndServe(config.Listen, nil))
}

// Custom flag type for multiple string values
type arrayFlags []string

func (a *arrayFlags) String() string {
	return strings.Join(*a, ",")
}

func (a *arrayFlags) Set(value string) error {
	*a = append(*a, value)
	return nil
}