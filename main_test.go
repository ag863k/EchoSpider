package main

import (
	"context"
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
}

func TestURLResolution(t *testing.T) {
	crawler := NewCrawler(1, 1, 5*time.Second, false)
	
	baseURL := "https://example.com/page"
	href := "/about"
	
	resolved := crawler.resolveURL(baseURL, href)
	expected := "https://example.com/about"
	
	if resolved != expected {
		t.Errorf("Expected %s, got %s", expected, resolved)
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
}

func TestRobotsChecker(t *testing.T) {
	checker := NewRobotsChecker()
	
	if checker.cache == nil {
		t.Error("Cache should be initialized")
	}
}
