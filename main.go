package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"echospider/pkg/echospider"
	"github.com/spf13/cobra"
)

// NewCrawler creates a new crawler instance with the specified configuration
func NewCrawler(maxDepth, maxWorkers int, timeout time.Duration, respectRobots bool) *echospider.Crawler {
	return echospider.NewCrawler(maxDepth, maxWorkers, timeout, respectRobots)
}
		}()
		return results
	}
	c.baseURL = baseURL
	
	go func() {
		defer close(results)
		
		jobs := make(chan Job, MaxJobQueueSize)
		var wg sync.WaitGroup
		
		for i := 0; i < c.maxWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for {
					select {
					case job, ok := <-jobs:
						if !ok {
							return
						}
						
						c.stats.incrementTotal()
						
						// Check if already visited and mark as visited atomically
						if !c.markVisited(job.URL) {
							c.stats.incrementSkipped()
							continue
						}
						
						result, err := c.crawlURL(ctx, job.URL)
						if err != nil {
							results <- CrawlResult{URL: job.URL, Error: err, Depth: job.Depth}
							continue
						}

						result.Depth = job.Depth
						results <- *result

						if job.Depth < c.maxDepth {
							for _, link := range result.Links {
								if !c.isVisited(link) {
									select {
									case jobs <- Job{URL: link, Depth: job.Depth + 1}:
									case <-ctx.Done():
										return
									}
								}
							}
						}
					case <-ctx.Done():
						return
					}
				}
			}(i)
		}
		
		jobs <- Job{URL: startURL, Depth: 0}
		
		go func() {
			wg.Wait()
			close(jobs)
		}()

		// Wait for context cancellation or all workers to finish
		select {
		case <-ctx.Done():
			// Context was cancelled, wait for workers to finish
			wg.Wait()
		case <-func() chan struct{} {
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()
			return done
		}():
			// All workers finished normally
		}
	}()
	
	return results
}

var rootCmd = &cobra.Command{
	Use:   "echospider [URL]",
	Short: "A professional concurrent web crawler",
	Long: `EchoSpider is a high-performance concurrent web crawler built with Go.
It demonstrates advanced concurrency patterns, context handling, and robust error management.

Features:
- Concurrent crawling with configurable worker pools
- Context-based timeout and cancellation
- Robots.txt compliance (optional)
- Professional error handling and statistics
- Internal link extraction and following`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		startURL := args[0]
		
		if _, err := url.Parse(startURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Invalid URL '%s': %v\n", startURL, err)
			os.Exit(1)
		}
		
		maxDepth, _ := cmd.Flags().GetInt("depth")
		maxWorkers, _ := cmd.Flags().GetInt("workers")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		respectRobots, _ := cmd.Flags().GetBool("respect-robots")
		verbose, _ := cmd.Flags().GetBool("verbose")
		
		if maxDepth < 0 {
			fmt.Fprintf(os.Stderr, "Error: depth must be >= 0\n")
			os.Exit(1)
		}
		if maxWorkers < MinWorkers || maxWorkers > MaxWorkers {
			fmt.Fprintf(os.Stderr, "Error: workers must be between %d and %d\n", MinWorkers, MaxWorkers)
			os.Exit(1)
		}
		
		crawler := NewCrawler(maxDepth, maxWorkers, timeout, respectRobots)
		
		totalTimeout := timeout * time.Duration(maxDepth*2+10)
		ctx, cancel := context.WithTimeout(context.Background(), totalTimeout)
		defer cancel()
		
		fmt.Println("EchoSpider - Professional Web Crawler")
		fmt.Println("------------------------------------------------------------")
		fmt.Printf("Starting crawl of: %s\n", startURL)
		fmt.Printf("Configuration: depth=%d, workers=%d, timeout=%v, robots=%t\n",
			maxDepth, maxWorkers, timeout, respectRobots)
		fmt.Printf("Runtime: Go %s on %s/%s with %d CPUs\n",
			runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
		fmt.Println("------------------------------------------------------------")
		
		results := crawler.Crawl(ctx, startURL)
		
		var totalResults int
		for result := range results {
			totalResults++
			
			if result.Error != nil {
				if verbose {
					fmt.Printf("Error crawling %s: %v\n", result.URL, result.Error)
				}
			} else if verbose {
				fmt.Printf("Crawled: %s\n", result.URL)
			}
		}

		// Get final stats
		stats := crawler.stats.getSnapshot()
		fmt.Println("\nCrawl Statistics")
		fmt.Println("------------------------------------------------------------")
		fmt.Printf("Total URLs processed: %d\n", stats.TotalURLs)
		fmt.Printf("Successful: %d\n", stats.SuccessfulURLs)
		fmt.Printf("Failed: %d\n", stats.FailedURLs)
		fmt.Printf("Skipped: %d\n", stats.SkippedURLs)
		if stats.BytesDownloaded > 0 {
			fmt.Printf("Data downloaded: %s\n", formatBytes(stats.BytesDownloaded))
		}
		fmt.Printf("Duration: %v\n", stats.EndTime.Sub(stats.StartTime).Round(time.Millisecond))
		if stats.TotalURLs > 0 {
			successRate := float64(stats.SuccessfulURLs) / float64(stats.TotalURLs) * 100
			fmt.Printf("Success rate: %.1f%%\n", successRate)
			avgTime := stats.EndTime.Sub(stats.StartTime) / time.Duration(stats.TotalURLs)
			fmt.Printf("Average time per URL: %v\n", avgTime.Round(time.Millisecond))
		}
	},
}

func init() {
	rootCmd.Flags().IntP("depth", "d", DefaultMaxDepth, "Maximum crawl depth")
	rootCmd.Flags().IntP("workers", "w", DefaultMaxWorkers, "Number of concurrent workers")
	rootCmd.Flags().DurationP("timeout", "t", DefaultTimeout, "Request timeout")
	rootCmd.Flags().BoolP("respect-robots", "r", false, "Respect robots.txt rules")
	rootCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
