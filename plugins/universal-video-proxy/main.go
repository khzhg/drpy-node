package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"universalvideoproxy/internal/cache"
	"universalvideoproxy/internal/config"
	"universalvideoproxy/internal/headers"
	"universalvideoproxy/internal/rewrite"
	"universalvideoproxy/internal/signer"
	"universalvideoproxy/internal/util"
)

type Server struct {
	config        *config.Config
	signer        *signer.Signer
	headerManager *headers.HeaderManager
	m3u8Cache     *cache.Cache
	keyCache      *cache.Cache
	tsCache       *cache.Cache
	httpClient    *http.Client
}

func main() {
	configPath := flag.String("config", "config.json", "Path to configuration file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	server := &Server{
		config:        cfg,
		signer:        signer.New(cfg.Sign.Secret, cfg.Sign.TTLSeconds, cfg.Sign.Enabled),
		headerManager: headers.New(cfg.Headers),
		m3u8Cache:     cache.New(cfg.Cache.M3U8.MaxEntries, cfg.Cache.M3U8.TTLSeconds, cfg.Cache.M3U8.Enabled),
		keyCache:      cache.New(cfg.Cache.Key.MaxEntries, cfg.Cache.Key.TTLSeconds, cfg.Cache.Key.Enabled),
		tsCache:       cache.New(cfg.Cache.TS.MaxEntries, cfg.Cache.TS.TTLSeconds, cfg.Cache.TS.Enabled),
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.Upstream.TimeoutMs) * time.Millisecond,
		},
	}

	mux := http.NewServeMux()
	
	// Setup routes
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/play", server.handlePlay)
	mux.HandleFunc("/seg", server.handleSegment)
	mux.HandleFunc("/key", server.handleKey)
	mux.HandleFunc("/raw", server.handleRaw)
	mux.HandleFunc("/sign", server.handleSign)

	// CORS middleware
	handler := server.corsMiddleware(mux)

	log.Printf("Universal Video Proxy starting on %s", cfg.Listen)
	log.Printf("Signing enabled: %v", cfg.Sign.Enabled)
	
	if err := http.ListenAndServe(cfg.Listen, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		if len(s.config.CORS.Origins) > 0 {
			origin := r.Header.Get("Origin")
			for _, allowed := range s.config.CORS.Origins {
				if allowed == "*" || allowed == origin {
					w.Header().Set("Access-Control-Allow-Origin", allowed)
					break
				}
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Accept-Ranges")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"time":   time.Now().Unix(),
	})
}

func (s *Server) handleSign(w http.ResponseWriter, r *http.Request) {
	if !s.signer.IsEnabled() {
		http.Error(w, "Signing is disabled", http.StatusForbidden)
		return
	}

	rawURL := r.URL.Query().Get("raw")
	if rawURL == "" {
		http.Error(w, "Missing 'raw' parameter", http.StatusBadRequest)
		return
	}

	rawURL = util.SanitizeURL(rawURL)
	if rawURL == "" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	if !util.IsAllowedHost(rawURL, s.config.AllowHosts) {
		http.Error(w, "Host not allowed", http.StatusForbidden)
		return
	}

	sign, ts, err := s.signer.Generate(rawURL)
	if err != nil {
		http.Error(w, "Failed to generate signature", http.StatusInternalServerError)
		return
	}

	proxyURL := fmt.Sprintf("/play?url=%s&sign=%s&ts=%s", 
		url.QueryEscape(rawURL), sign, ts)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"proxy": proxyURL,
		"sign":  sign,
		"ts":    ts,
	})
}

func (s *Server) handlePlay(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		http.Error(w, "Missing 'url' parameter", http.StatusBadRequest)
		return
	}

	targetURL = util.SanitizeURL(targetURL)
	if targetURL == "" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	if len(targetURL) > s.config.Limits.MaxURLLength {
		http.Error(w, "URL too long", http.StatusBadRequest)
		return
	}

	if !util.IsAllowedHost(targetURL, s.config.AllowHosts) {
		http.Error(w, "Host not allowed", http.StatusForbidden)
		return
	}

	// Verify signature if enabled
	sign := r.URL.Query().Get("sign")
	ts := r.URL.Query().Get("ts")
	if err := s.signer.Verify(targetURL, sign, ts); err != nil {
		http.Error(w, "Signature verification failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Check cache for M3U8
	cacheKey := targetURL
	if cached, found := s.m3u8Cache.Get(cacheKey); found {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(cached)
		return
	}

	// Fetch content
	content, contentType, err := s.fetchContent(targetURL)
	if err != nil {
		log.Printf("Failed to fetch %s: %v", targetURL, err)
		http.Error(w, "Failed to fetch content", http.StatusBadGateway)
		return
	}

	// Check if it's M3U8 and should be rewritten
	if s.config.Rewrite.EnableM3U8 && rewrite.IsM3U8Content(content) {
		// Build sign params for rewritten URLs
		signParams := util.BuildSignParams(sign, ts)
		
		// Rewrite the playlist
		rewriter := rewrite.NewM3U8Rewriter("", "/seg", "/key", signParams)
		rewritten, err := rewriter.Rewrite(content, targetURL)
		if err != nil {
			log.Printf("Failed to rewrite M3U8: %v", err)
			http.Error(w, "Failed to process playlist", http.StatusInternalServerError)
			return
		}

		// Cache the rewritten content
		s.m3u8Cache.Set(cacheKey, rewritten)

		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(rewritten)
		return
	}

	// For non-M3U8 content, stream directly with range support
	s.streamContent(w, r, targetURL, content, contentType)
}

func (s *Server) handleSegment(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("u")
	if targetURL == "" {
		http.Error(w, "Missing 'u' parameter", http.StatusBadRequest)
		return
	}

	targetURL = util.SanitizeURL(targetURL)
	if targetURL == "" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	if !util.IsAllowedHost(targetURL, s.config.AllowHosts) {
		http.Error(w, "Host not allowed", http.StatusForbidden)
		return
	}

	// Verify signature if enabled
	sign := r.URL.Query().Get("sign")
	ts := r.URL.Query().Get("ts")
	if err := s.signer.Verify(targetURL, sign, ts); err != nil {
		http.Error(w, "Signature verification failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Check TS cache
	cacheKey := targetURL
	if cached, found := s.tsCache.Get(cacheKey); found {
		w.Header().Set("Content-Type", util.GetContentType(targetURL))
		w.Header().Set("Accept-Ranges", "bytes")
		w.Write(cached)
		return
	}

	// Fetch and stream segment
	content, contentType, err := s.fetchContent(targetURL)
	if err != nil {
		log.Printf("Failed to fetch segment %s: %v", targetURL, err)
		http.Error(w, "Failed to fetch segment", http.StatusBadGateway)
		return
	}

	// Cache if enabled and not too large
	if s.tsCache.IsEnabled() && len(content) < 1024*1024 { // Cache only if < 1MB
		s.tsCache.Set(cacheKey, content)
	}

	s.streamContent(w, r, targetURL, content, contentType)
}

func (s *Server) handleKey(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("u")
	if targetURL == "" {
		http.Error(w, "Missing 'u' parameter", http.StatusBadRequest)
		return
	}

	targetURL = util.SanitizeURL(targetURL)
	if targetURL == "" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	if !util.IsAllowedHost(targetURL, s.config.AllowHosts) {
		http.Error(w, "Host not allowed", http.StatusForbidden)
		return
	}

	// Verify signature if enabled
	sign := r.URL.Query().Get("sign")
	ts := r.URL.Query().Get("ts")
	if err := s.signer.Verify(targetURL, sign, ts); err != nil {
		http.Error(w, "Signature verification failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Check key cache
	cacheKey := targetURL
	if cached, found := s.keyCache.Get(cacheKey); found {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(cached)
		return
	}

	// Fetch key
	content, _, err := s.fetchContent(targetURL)
	if err != nil {
		log.Printf("Failed to fetch key %s: %v", targetURL, err)
		http.Error(w, "Failed to fetch key", http.StatusBadGateway)
		return
	}

	// Cache the key
	s.keyCache.Set(cacheKey, content)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(content)
}

func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		http.Error(w, "Missing 'url' parameter", http.StatusBadRequest)
		return
	}

	targetURL = util.SanitizeURL(targetURL)
	if targetURL == "" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	if !util.IsAllowedHost(targetURL, s.config.AllowHosts) {
		http.Error(w, "Host not allowed", http.StatusForbidden)
		return
	}

	// Fetch and stream raw content (no caching, no rewriting)
	content, contentType, err := s.fetchContent(targetURL)
	if err != nil {
		log.Printf("Failed to fetch raw %s: %v", targetURL, err)
		http.Error(w, "Failed to fetch content", http.StatusBadGateway)
		return
	}

	s.streamContent(w, r, targetURL, content, contentType)
}

func (s *Server) fetchContent(targetURL string) ([]byte, string, error) {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, "", err
	}

	// Process headers
	originalHeaders := map[string]string{
		"User-Agent": "Mozilla/5.0 (compatible; UniversalVideoProxy/1.0)",
	}
	processedHeaders := s.headerManager.ProcessHeaders(targetURL, originalHeaders)
	
	for k, v := range processedHeaders {
		req.Header.Set(k, v)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("upstream returned %d", resp.StatusCode)
	}

	// Check content length limits for playlists
	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		if length, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
			if length > int64(s.config.Limits.MaxPlaylistKB*1024) {
				return nil, "", fmt.Errorf("content too large: %d bytes", length)
			}
		}
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = util.GetContentType(targetURL)
	}

	return content, contentType, nil
}

func (s *Server) streamContent(w http.ResponseWriter, r *http.Request, targetURL string, content []byte, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))

	// Handle range requests
	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" {
		s.handleRangeRequest(w, r, content, rangeHeader)
		return
	}

	w.Write(content)
}

func (s *Server) handleRangeRequest(w http.ResponseWriter, r *http.Request, content []byte, rangeHeader string) {
	contentLength := len(content)
	
	// Parse range header
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}

	rangeSpec := rangeHeader[6:] // Remove "bytes="
	parts := strings.Split(rangeSpec, "-")
	if len(parts) != 2 {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}

	var start, end int
	var err error

	if parts[0] != "" {
		start, err = strconv.Atoi(parts[0])
		if err != nil || start < 0 {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
	}

	if parts[1] != "" {
		end, err = strconv.Atoi(parts[1])
		if err != nil || end >= contentLength {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
	} else {
		end = contentLength - 1
	}

	if start > end {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}

	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, contentLength))
	w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
	w.WriteHeader(http.StatusPartialContent)
	w.Write(content[start : end+1])
}