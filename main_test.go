package main

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCrawlerCreation(t *testing.T) {
	crawler := NewCrawler(2, 10, 10*time.Second, false)
	
	if crawler.maxDepth != 2 {
		t.Errorf("Expected maxDepth 2, got %d", crawler.maxDepth)
	}
	
	if crawler.maxWorkers != 10 {
		t.Errorf("Expected maxWorkers 10, got %d", crawler.maxWorkers)
	}
	
	if crawler.timeout != 10*time.Second {
		t.Errorf("Expected timeout 10s, got %v", crawler.timeout)
	}
	
	if crawler.visited == nil {
		t.Error("Visited map should be initialized")
	}
	
	if crawler.client == nil {
		t.Error("HTTP client should be initialized")
	}
	
	if crawler.robotsChecker == nil {
		t.Error("Robots checker should be initialized")
	}
}

func TestURLResolution(t *testing.T) {
	crawler := NewCrawler(1, 1, 5*time.Second, false)
	
	testCases := []struct {
		name     string
		baseURL  string
		href     string
		expected string
	}{
		{
			name:     "Absolute path",
			baseURL:  "https://example.com/page",
			href:     "/about",
			expected: "https://example.com/about",
		},
		{
			name:     "Relative path",
			baseURL:  "https://example.com/dir/page",
			href:     "about",
			expected: "https://example.com/dir/about",
		},
		{
			name:     "Full URL",
			baseURL:  "https://example.com/page",
			href:     "https://example.com/contact",
			expected: "https://example.com/contact",
		},
		{
			name:     "Query parameters",
			baseURL:  "https://example.com/page",
			href:     "search?q=test",
			expected: "https://example.com/search?q=test",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resolved := crawler.resolveURL(tc.baseURL, tc.href)
			if resolved != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, resolved)
			}
		})
	}
}

func TestVisitedTracking(t *testing.T) {
	crawler := NewCrawler(1, 1, 5*time.Second, false)
	
	url := "https://example.com"
	
	if crawler.isVisited(url) {
		t.Error("URL should not be visited initially")
	}
	
	crawler.markVisited(url)
	
	if !crawler.isVisited(url) {
		t.Error("URL should be marked as visited")
	}
	
	// Test multiple URLs
	urls := []string{
		"https://example.com/page1",
		"https://example.com/page2",
		"https://example.com/page3",
	}
	
	for _, testURL := range urls {
		if crawler.isVisited(testURL) {
			t.Errorf("URL %s should not be visited initially", testURL)
		}
		crawler.markVisited(testURL)
		if !crawler.isVisited(testURL) {
			t.Errorf("URL %s should be marked as visited", testURL)
		}
	}
}

func TestInternalLinkDetection(t *testing.T) {
	crawler := NewCrawler(1, 1, 5*time.Second, false)
	
	// Set base URL
	baseURL, _ := url.Parse("https://example.com")
	crawler.baseURL = baseURL
	
	testCases := []struct {
		name     string
		link     string
		expected bool
	}{
		{
			name:     "Same domain",
			link:     "https://example.com/page",
			expected: true,
		},
		{
			name:     "Different domain",
			link:     "https://other.com/page",
			expected: false,
		},
		{
			name:     "Subdomain",
			link:     "https://sub.example.com/page",
			expected: false,
		},
		{
			name:     "Invalid URL",
			link:     "://invalid",
			expected: false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := crawler.isInternalLink(tc.link)
			if result != tc.expected {
				t.Errorf("Expected %t for %s, got %t", tc.expected, tc.link, result)
			}
		})
	}
}

func TestRobotsChecker(t *testing.T) {
	checker := NewRobotsChecker()
	
	if checker.cache == nil {
		t.Error("Cache should be initialized")
	}
	
	// Test robots.txt parsing
	testRobotsContent := `User-agent: *
Disallow: /private/
Disallow: /admin/
Allow: /public/

User-agent: TestBot
Disallow: /
Crawl-delay: 5`
	
	rules := &RobotsRules{
		userAgents: make(map[string][]string),
		crawlDelay: make(map[string]int),
	}
	
	// Parse test content
	lines := strings.Split(testRobotsContent, "\n")
	var currentUserAgent string
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
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
		}
	}
	
	// Test rule checking
	if len(rules.userAgents["*"]) == 0 {
		t.Error("Should have rules for * user agent")
	}
	
	if len(rules.userAgents["TestBot"]) == 0 {
		t.Error("Should have rules for TestBot user agent")
	}
}

func TestCrawlResultStructure(t *testing.T) {
	result := CrawlResult{
		URL:   "https://example.com",
		Title: "Example Domain",
		Links: []string{"https://example.com/about", "https://example.com/contact"},
		Depth: 1,
		Error: nil,
	}
	
	if result.URL != "https://example.com" {
		t.Errorf("Expected URL to be https://example.com, got %s", result.URL)
	}
	
	if len(result.Links) != 2 {
		t.Errorf("Expected 2 links, got %d", len(result.Links))
	}
	
	if result.Depth != 1 {
		t.Errorf("Expected depth 1, got %d", result.Depth)
	}
}
