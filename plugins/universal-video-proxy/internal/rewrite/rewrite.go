package rewrite

import (
	"bufio"
	"fmt"
	"net/url"
	"strings"
)

type M3U8Rewriter struct {
	baseURL    string
	segPrefix  string
	keyPrefix  string
	signParams string
}

func NewM3U8Rewriter(baseURL, segPrefix, keyPrefix, signParams string) *M3U8Rewriter {
	return &M3U8Rewriter{
		baseURL:    baseURL,
		segPrefix:  segPrefix,
		keyPrefix:  keyPrefix,
		signParams: signParams,
	}
}

func (r *M3U8Rewriter) Rewrite(content []byte, originalURL string) ([]byte, error) {
	lines := strings.Split(string(content), "\n")
	var result []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		if line == "" || strings.HasPrefix(line, "#") {
			// Handle special directives
			if strings.HasPrefix(line, "#EXT-X-KEY:") {
				line = r.rewriteKeyLine(line, originalURL)
			}
			result = append(result, line)
		} else {
			// This is a segment URL
			segmentURL := r.resolveURL(line, originalURL)
			proxyURL := r.buildSegmentURL(segmentURL)
			result = append(result, proxyURL)
		}
	}

	return []byte(strings.Join(result, "\n")), nil
}

func (r *M3U8Rewriter) rewriteKeyLine(line, baseURL string) string {
	// Extract URI from EXT-X-KEY line
	parts := strings.Split(line, ",")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "URI=") {
			// Extract the quoted URI
			uriPart := part[4:] // Remove "URI="
			if len(uriPart) >= 2 && uriPart[0] == '"' && uriPart[len(uriPart)-1] == '"' {
				uri := uriPart[1 : len(uriPart)-1] // Remove quotes
				resolvedURI := r.resolveURL(uri, baseURL)
				proxyURI := r.buildKeyURL(resolvedURI)
				parts[i] = fmt.Sprintf("URI=\"%s\"", proxyURI)
			}
		}
	}
	return strings.Join(parts, ",")
}

func (r *M3U8Rewriter) resolveURL(urlStr, baseURL string) string {
	if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
		return urlStr
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return urlStr
	}

	resolved, err := base.Parse(urlStr)
	if err != nil {
		return urlStr
	}

	return resolved.String()
}

func (r *M3U8Rewriter) buildSegmentURL(segmentURL string) string {
	encoded := url.QueryEscape(segmentURL)
	result := r.baseURL + r.segPrefix + "?u=" + encoded
	if r.signParams != "" {
		result += "&" + r.signParams
	}
	return result
}

func (r *M3U8Rewriter) buildKeyURL(keyURL string) string {
	encoded := url.QueryEscape(keyURL)
	result := r.baseURL + r.keyPrefix + "?u=" + encoded
	if r.signParams != "" {
		result += "&" + r.signParams
	}
	return result
}

func IsM3U8Content(content []byte) bool {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#EXTM3U") {
			return true
		}
		if line != "" && !strings.HasPrefix(line, "#") {
			// If we hit non-comment content before #EXTM3U, it's probably not M3U8
			break
		}
	}
	return false
}