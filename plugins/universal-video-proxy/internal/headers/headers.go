package headers

import (
	"net/url"
	"regexp"
	"strings"
	"universalvideoproxy/internal/config"
)

type HeaderManager struct {
	rules []config.HeaderRule
}

func New(rules []config.HeaderRule) *HeaderManager {
	return &HeaderManager{rules: rules}
}

func (hm *HeaderManager) ProcessHeaders(targetURL string, originalHeaders map[string]string) map[string]string {
	result := make(map[string]string)
	
	// Copy original headers
	for k, v := range originalHeaders {
		result[k] = v
	}

	parsed, err := url.Parse(targetURL)
	if err != nil {
		return result
	}

	hostname := parsed.Hostname()

	// Apply matching rules
	for _, rule := range hm.rules {
		if hm.matchesRule(hostname, rule) {
			// Apply header overrides
			for k, v := range rule.Set {
				result[k] = v
			}
			
			// Handle host rewrite
			if rule.HostRewrite {
				result["Host"] = hostname
			}
		}
	}

	return result
}

func (hm *HeaderManager) matchesRule(hostname string, rule config.HeaderRule) bool {
	if rule.UseRegex {
		if matched, err := regexp.MatchString(rule.Match, hostname); err == nil && matched {
			return true
		}
	} else {
		if strings.Contains(hostname, rule.Match) {
			return true
		}
	}
	return false
}