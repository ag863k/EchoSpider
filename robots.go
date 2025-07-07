package main

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RobotsChecker struct {
	cache    map[string]*RobotsRules
	cacheMu  sync.RWMutex
	client   *http.Client
}

type RobotsRules struct {
	userAgents map[string][]string
	crawlDelay map[string]int
	sitemaps   []string
	timestamp  time.Time
}

func NewRobotsChecker() *RobotsChecker {
	return &RobotsChecker{
		cache: make(map[string]*RobotsRules),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (rc *RobotsChecker) CanCrawl(targetURL, userAgent string) bool {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return false
	}

	baseURL := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
	
	// Check cache first
	rc.cacheMu.RLock()
	if rules, exists := rc.cache[baseURL]; exists {
		// Check if cache is still valid (24 hours)
		if time.Since(rules.timestamp) < 24*time.Hour {
			rc.cacheMu.RUnlock()
			return rc.checkRules(rules, parsedURL.Path, userAgent)
		}
	}
	rc.cacheMu.RUnlock()

	// Fetch new rules
	rules := rc.fetchRobotsTxt(baseURL)
	
	// Update cache
	rc.cacheMu.Lock()
	rc.cache[baseURL] = rules
	rc.cacheMu.Unlock()
	
	return rc.checkRules(rules, parsedURL.Path, userAgent)
}

func (rc *RobotsChecker) fetchRobotsTxt(baseURL string) *RobotsRules {
	robotsURL := baseURL + "/robots.txt"
	
	resp, err := rc.client.Get(robotsURL)
	if err != nil {
		// If robots.txt is not accessible, allow crawling
		return &RobotsRules{
			userAgents: make(map[string][]string),
			crawlDelay: make(map[string]int),
			sitemaps:   make([]string, 0),
			timestamp:  time.Now(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// If robots.txt returns non-200, allow crawling
		return &RobotsRules{
			userAgents: make(map[string][]string),
			crawlDelay: make(map[string]int),
			sitemaps:   make([]string, 0),
			timestamp:  time.Now(),
		}
	}

	return rc.parseRobotsTxt(resp)
}

func (rc *RobotsChecker) parseRobotsTxt(resp *http.Response) *RobotsRules {
	rules := &RobotsRules{
		userAgents: make(map[string][]string),
		crawlDelay: make(map[string]int),
		sitemaps:   make([]string, 0),
		timestamp:  time.Now(),
	}

	scanner := bufio.NewScanner(resp.Body)
	var currentUserAgent string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Find the colon separator
		colonIndex := strings.Index(line, ":")
		if colonIndex == -1 {
			continue
		}

		directive := strings.ToLower(strings.TrimSpace(line[:colonIndex]))
		value := strings.TrimSpace(line[colonIndex+1:])

		switch directive {
		case "user-agent":
			currentUserAgent = value
		case "disallow":
			if currentUserAgent != "" {
				rules.userAgents[currentUserAgent] = append(rules.userAgents[currentUserAgent], value)
			}
		case "allow":
			if currentUserAgent != "" {
				// For allow rules, we prefix with "ALLOW:" to distinguish from disallow
				rules.userAgents[currentUserAgent] = append(rules.userAgents[currentUserAgent], "ALLOW:"+value)
			}
		case "crawl-delay":
			if currentUserAgent != "" {
				if delay, err := strconv.Atoi(value); err == nil {
					rules.crawlDelay[currentUserAgent] = delay
				}
			}
		case "sitemap":
			rules.sitemaps = append(rules.sitemaps, value)
		}
	}

	return rules
}

func (rc *RobotsChecker) checkRules(rules *RobotsRules, path, userAgent string) bool {
	// Get rules for the specific user agent, fall back to wildcard
	var applicableRules []string
	
	if agentRules, exists := rules.userAgents[userAgent]; exists {
		applicableRules = agentRules
	} else if wildcardRules, exists := rules.userAgents["*"]; exists {
		applicableRules = wildcardRules
	} else {
		// No rules found, allow crawling
		return true
	}

	// Check rules in order
	for _, rule := range applicableRules {
		if strings.HasPrefix(rule, "ALLOW:") {
			// Allow rule
			allowPattern := strings.TrimPrefix(rule, "ALLOW:")
			if rc.matchesPattern(path, allowPattern) {
				return true
			}
		} else {
			// Disallow rule
			if rc.matchesPattern(path, rule) {
				return false
			}
		}
	}

	// If no rules matched, allow crawling
	return true
}

func (rc *RobotsChecker) matchesPattern(path, pattern string) bool {
	if pattern == "" {
		return false
	}
	
	// Empty pattern matches nothing
	if pattern == "/" {
		return true
	}
	
	// Simple prefix matching (robots.txt uses prefix matching)
	return strings.HasPrefix(path, pattern)
}

func (rc *RobotsChecker) GetCrawlDelay(targetURL, userAgent string) time.Duration {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return 0
	}

	baseURL := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
	
	rc.cacheMu.RLock()
	defer rc.cacheMu.RUnlock()
	
	if rules, exists := rc.cache[baseURL]; exists {
		if delay, exists := rules.crawlDelay[userAgent]; exists {
			return time.Duration(delay) * time.Second
		}
		if delay, exists := rules.crawlDelay["*"]; exists {
			return time.Duration(delay) * time.Second
		}
	}
	
	return 0
}

func (rc *RobotsChecker) GetSitemaps(targetURL string) []string {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return nil
	}

	baseURL := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
	
	rc.cacheMu.RLock()
	defer rc.cacheMu.RUnlock()
	
	if rules, exists := rc.cache[baseURL]; exists {
		return rules.sitemaps
	}
	
	return nil
}
