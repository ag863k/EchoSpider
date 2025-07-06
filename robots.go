package main

import (
	"bufio"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type RobotsChecker struct {
	cache map[string]*RobotsRules
}

type RobotsRules struct {
	userAgents map[string][]string
	crawlDelay map[string]int
}

func NewRobotsChecker() *RobotsChecker {
	return &RobotsChecker{
		cache: make(map[string]*RobotsRules),
	}
}

func (rc *RobotsChecker) CanCrawl(targetURL, userAgent string) bool {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return false
	}

	baseURL := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
	
	if rules, exists := rc.cache[baseURL]; exists {
		return rc.checkRules(rules, parsedURL.Path, userAgent)
	}

	rules := rc.fetchRobotsTxt(baseURL)
	rc.cache[baseURL] = rules
	
	return rc.checkRules(rules, parsedURL.Path, userAgent)
}

func (rc *RobotsChecker) fetchRobotsTxt(baseURL string) *RobotsRules {
	robotsURL := baseURL + "/robots.txt"
	
	resp, err := http.Get(robotsURL)
	if err != nil {
		return &RobotsRules{
			userAgents: make(map[string][]string),
			crawlDelay: make(map[string]int),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &RobotsRules{
			userAgents: make(map[string][]string),
			crawlDelay: make(map[string]int),
		}
	}

	return rc.parseRobotsTxt(resp)
}

func (rc *RobotsChecker) parseRobotsTxt(resp *http.Response) *RobotsRules {
	rules := &RobotsRules{
		userAgents: make(map[string][]string),
		crawlDelay: make(map[string]int),
	}

	scanner := bufio.NewScanner(resp.Body)
	var currentUserAgent string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		directive := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch directive {
		case "user-agent":
			currentUserAgent = value
		case "disallow":
			if currentUserAgent != "" {
				rules.userAgents[currentUserAgent] = append(rules.userAgents[currentUserAgent], value)
			}
		case "crawl-delay":
			if currentUserAgent != "" {
				var delay int
				fmt.Sscanf(value, "%d", &delay)
				rules.crawlDelay[currentUserAgent] = delay
			}
		}
	}

	return rules
}

func (rc *RobotsChecker) checkRules(rules *RobotsRules, path, userAgent string) bool {
	disallowed := rules.userAgents[userAgent]
	if len(disallowed) == 0 {
		disallowed = rules.userAgents["*"]
	}

	for _, pattern := range disallowed {
		if pattern == "/" {
			return false
		}
		if strings.HasPrefix(path, pattern) {
			return false
		}
	}

	return true
}
