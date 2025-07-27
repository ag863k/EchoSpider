package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Example demonstrates how to use EchoSpider programmatically
func ExampleUsage() {
	// Create a new crawler
	crawler := NewCrawler(
		2,                // maxDepth
		5,                // maxWorkers
		10*time.Second,   // timeout
		true,             // respectRobots
	)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Start crawling
	fmt.Println("Starting crawl of https://httpbin.org")
	results := crawler.Crawl(ctx, "https://httpbin.org")

	// Process results
	var successCount, errorCount int
	for result := range results {
		if result.Error != nil {
			errorCount++
			fmt.Printf("ERROR: %s - %v\n", result.URL, result.Error)
			continue
		}

		successCount++
		fmt.Printf("SUCCESS [Depth:%d] %s\n", result.Depth, result.URL)
		if result.Title != "" {
			fmt.Printf("  Title: %s\n", result.Title)
		}
		if len(result.Links) > 0 {
			fmt.Printf("  Links: %d found\n", len(result.Links))
		}
	}

	// Print final statistics
	fmt.Printf("\nFinal Statistics:\n")
	fmt.Printf("  Successful: %d\n", successCount)
	fmt.Printf("  Errors: %d\n", errorCount)
	fmt.Printf("  Total: %d\n", successCount+errorCount)
}

// ExampleRobotsChecker demonstrates robots.txt checking
func ExampleRobotsChecker() {
	checker := NewRobotsChecker()

	// Check if we can crawl a URL
	canCrawl := checker.CanCrawl("https://httpbin.org/robots.txt", "EchoSpider/1.0")
	fmt.Printf("Can crawl httpbin.org: %t\n", canCrawl)

	// Get crawl delay
	delay := checker.GetCrawlDelay("https://httpbin.org", "EchoSpider/1.0")
	fmt.Printf("Crawl delay: %v\n", delay)

	// Get sitemaps
	sitemaps := checker.GetSitemaps("https://httpbin.org")
	fmt.Printf("Sitemaps: %v\n", sitemaps)
}

// ExampleWithCustomContext demonstrates context usage
func ExampleWithCustomContext() {
	crawler := NewCrawler(1, 3, 5*time.Second, false)

	// Create context that cancels after 30 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start crawling
	results := crawler.Crawl(ctx, "https://httpbin.org")

	// Process results with timeout handling
	for {
		select {
		case result, ok := <-results:
			if !ok {
				fmt.Println("Crawl completed")
				return
			}
			
			if result.Error != nil {
				fmt.Printf("Error: %v\n", result.Error)
				continue
			}
			
			fmt.Printf("Crawled: %s\n", result.URL)
			
		case <-ctx.Done():
			fmt.Println("Crawl timed out")
			return
		}
	}
}

// This file demonstrates usage but is not part of the main program
func init() {
	// Uncomment to run examples
	// ExampleUsage()
	// ExampleRobotsChecker()
	// ExampleWithCustomContext()
}

func main() {
	log.Println("This is the example file. Run 'go run main.go robots.go config.go' to use the actual crawler.")
}
