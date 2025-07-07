package main

import (
	"context"
	"fmt"
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

	if crawler.client == nil {
		t.Error("HTTP client should be initialized")
	}

	if crawler.robotsChecker == nil {
		t.Error("Robots checker should be initialized")
	}

	if crawler.stats == nil {
		t.Error("Stats should be initialized")
	}

	// Test parameter validation
	crawlerMinWorkers := NewCrawler(1, 0, 5*time.Second, false) // Should adjust to MinWorkers
	if crawlerMinWorkers.maxWorkers < MinWorkers {
		t.Errorf("Expected workers to be adjusted to minimum %d, got %d", MinWorkers, crawlerMinWorkers.maxWorkers)
	}

	crawlerMaxWorkers := NewCrawler(1, 1000, 5*time.Second, false) // Should adjust to MaxWorkers
	if crawlerMaxWorkers.maxWorkers > MaxWorkers {
		t.Errorf("Expected workers to be adjusted to maximum %d, got %d", MaxWorkers, crawlerMaxWorkers.maxWorkers)
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

	// Test atomic markVisited - should return true for first time
	if !crawler.markVisited(url) {
		t.Error("markVisited should return true for first time")
	}

	if !crawler.isVisited(url) {
		t.Error("URL should be marked as visited")
	}

	// Test atomic markVisited - should return false for second time
	if crawler.markVisited(url) {
		t.Error("markVisited should return false for second time")
	}

	// Test multiple URLs concurrently
	urls := []string{
		"https://example.com/page1",
		"https://example.com/page2",
		"https://example.com/page3",
	}

	var wg sync.WaitGroup
	for _, testURL := range urls {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			if crawler.isVisited(u) {
				t.Errorf("URL %s should not be visited initially", u)
			}
			if !crawler.markVisited(u) {
				t.Errorf("markVisited should return true for first time for %s", u)
			}
			if !crawler.isVisited(u) {
				t.Errorf("URL %s should be marked as visited", u)
			}
		}(testURL)
	}
	wg.Wait()
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

func TestCrawlStatsTracking(t *testing.T) {
	crawler := NewCrawler(1, 1, 5*time.Second, false)

	// Test atomic operations
	crawler.stats.incrementTotal()
	crawler.stats.incrementSuccessful()
	crawler.stats.incrementFailed()
	crawler.stats.incrementSkipped()
	crawler.stats.addBytes(1024)

	stats := crawler.stats.getSnapshot()

	if stats.TotalURLs != 1 {
		t.Errorf("Expected TotalURLs 1, got %d", stats.TotalURLs)
	}
	if stats.SuccessfulURLs != 1 {
		t.Errorf("Expected SuccessfulURLs 1, got %d", stats.SuccessfulURLs)
	}
	if stats.FailedURLs != 1 {
		t.Errorf("Expected FailedURLs 1, got %d", stats.FailedURLs)
	}
	if stats.SkippedURLs != 1 {
		t.Errorf("Expected SkippedURLs 1, got %d", stats.SkippedURLs)
	}
	if stats.BytesDownloaded != 1024 {
		t.Errorf("Expected BytesDownloaded 1024, got %d", stats.BytesDownloaded)
	}
}

func TestRetryableError(t *testing.T) {
	crawler := NewCrawler(1, 1, 5*time.Second, false)

	testCases := []struct {
		name      string
		error     string
		retryable bool
	}{
		{"timeout error", "connection timeout", true},
		{"connection refused", "connection refused", true},
		{"server closed", "server closed connection", true},
		{"broken pipe", "broken pipe", true},
		{"not found", "404 not found", false},
		{"forbidden", "403 forbidden", false},
		{"nil error", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.error != "" {
				err = fmt.Errorf(tc.error)
			}
			result := crawler.isRetryableError(err)
			if result != tc.retryable {
				t.Errorf("Expected %t for error '%s', got %t", tc.retryable, tc.error, result)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	testCases := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%d bytes", tc.bytes), func(t *testing.T) {
			result := formatBytes(tc.bytes)
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestMinFunction(t *testing.T) {
	testCases := []struct {
		a, b, expected int
	}{
		{1, 2, 1},
		{5, 3, 3},
		{0, 0, 0},
		{-1, 1, -1},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("min(%d,%d)", tc.a, tc.b), func(t *testing.T) {
			result := min(tc.a, tc.b)
			if result != tc.expected {
				t.Errorf("Expected %d, got %d", tc.expected, result)
			}
		})
	}
}
