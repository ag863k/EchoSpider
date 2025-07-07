package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/net/html"
)

const (
	UserAgent = "EchoSpider/1.0"
	MaxJobQueueSize = 1000
	MaxResultQueueSize = 100
)

type Crawler struct {
	maxDepth      int
	maxWorkers    int
	timeout       time.Duration
	visited       map[string]bool
	visitedMu     sync.RWMutex
	client        *http.Client
	respectRobots bool
	baseURL       *url.URL
	robotsChecker *RobotsChecker
	stats         *CrawlStats
	statsMu       sync.RWMutex
}

type CrawlStats struct {
	TotalURLs     int
	SuccessfulURLs int
	FailedURLs    int
	StartTime     time.Time
	EndTime       time.Time
}

type CrawlResult struct {
	URL    string
	Title  string
	Links  []string
	Depth  int
	Error  error
	Stats  *CrawlStats
}

func NewCrawler(maxDepth, maxWorkers int, timeout time.Duration, respectRobots bool) *Crawler {
	return &Crawler{
		maxDepth:      maxDepth,
		maxWorkers:    maxWorkers,
		timeout:       timeout,
		visited:       make(map[string]bool),
		respectRobots: respectRobots,
		robotsChecker: NewRobotsChecker(),
		stats: &CrawlStats{
			StartTime: time.Now(),
		},
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:       100,
				IdleConnTimeout:    90 * time.Second,
				DisableCompression: false,
			},
		},
	}
}

func (c *Crawler) updateStats(success bool) {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	
	c.stats.TotalURLs++
	if success {
		c.stats.SuccessfulURLs++
	} else {
		c.stats.FailedURLs++
	}
}

func (c *Crawler) getStats() *CrawlStats {
	c.statsMu.RLock()
	defer c.statsMu.RUnlock()
	
	stats := *c.stats
	stats.EndTime = time.Now()
	return &stats
}

func (c *Crawler) isVisited(url string) bool {
	c.visitedMu.RLock()
	defer c.visitedMu.RUnlock()
	return c.visited[url]
}

func (c *Crawler) markVisited(url string) {
	c.visitedMu.Lock()
	defer c.visitedMu.Unlock()
	c.visited[url] = true
}

func (c *Crawler) crawlURL(ctx context.Context, rawURL string) (*CrawlResult, error) {
	if c.respectRobots && !c.robotsChecker.CanCrawl(rawURL, UserAgent) {
		return nil, fmt.Errorf("robots.txt disallows crawling")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Connection", "keep-alive")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(contentType), "text/html") {
		return nil, fmt.Errorf("not HTML content: %s", contentType)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	title := c.extractTitle(doc)
	links := c.extractLinks(doc, rawURL)

	return &CrawlResult{
		URL:   rawURL,
		Title: title,
		Links: links,
		Stats: c.getStats(),
	}, nil
}

func (c *Crawler) extractTitle(doc *html.Node) string {
	var title string
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "title" {
			if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
				title = strings.TrimSpace(n.FirstChild.Data)
				return
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
			if title != "" {
				return
			}
		}
	}
	traverse(doc)
	return title
}

func (c *Crawler) extractLinks(doc *html.Node, baseURL string) []string {
	var links []string
	linkMap := make(map[string]bool)
	
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					link := c.resolveURL(baseURL, attr.Val)
					if link != "" && c.isInternalLink(link) && !linkMap[link] {
						links = append(links, link)
						linkMap[link] = true
					}
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}
	traverse(doc)
	return links
}

func (c *Crawler) resolveURL(baseURL, href string) string {
	if href == "" {
		return ""
	}
	
	// Skip non-HTTP(S) links
	if strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") || 
	   strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "ftp:") {
		return ""
	}
	
	// Skip fragments only
	if strings.HasPrefix(href, "#") {
		return ""
	}
	
	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	
	link, err := url.Parse(href)
	if err != nil {
		return ""
	}
	
	resolved := base.ResolveReference(link)
	
	// Remove fragment
	resolved.Fragment = ""
	
	return resolved.String()
}

func (c *Crawler) isInternalLink(link string) bool {
	if c.baseURL == nil {
		return true
	}
	
	linkURL, err := url.Parse(link)
	if err != nil {
		return false
	}
	
	return linkURL.Host == c.baseURL.Host
}

func (c *Crawler) Crawl(ctx context.Context, startURL string) <-chan CrawlResult {
	results := make(chan CrawlResult, MaxResultQueueSize)
	
	baseURL, err := url.Parse(startURL)
	if err != nil {
		go func() {
			defer close(results)
			results <- CrawlResult{URL: startURL, Error: fmt.Errorf("parsing start URL: %w", err)}
		}()
		return results
	}
	c.baseURL = baseURL
	
	go func() {
		defer close(results)
		
		type job struct {
			url   string
			depth int
		}
		
		jobs := make(chan job, MaxJobQueueSize)
		var wg sync.WaitGroup
		
		// Start workers
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
						
						if c.isVisited(job.url) {
							continue
						}
						c.markVisited(job.url)
						
						result, err := c.crawlURL(ctx, job.url)
						if err != nil {
							c.updateStats(false)
							results <- CrawlResult{URL: job.url, Depth: job.depth, Error: err}
							continue
						}
						
						c.updateStats(true)
						result.Depth = job.depth
						results <- *result
						
						// Queue new links if within depth limit
						if job.depth < c.maxDepth {
							for _, link := range result.Links {
								if !c.isVisited(link) {
									select {
									case jobs <- job{url: link, depth: job.depth + 1}:
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
		
		// Start crawling
		jobs <- job{url: startURL, depth: 0}
		
		// Wait for all workers to finish
		go func() {
			wg.Wait()
			close(jobs)
		}()
		
		wg.Wait()
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
		
		// Validate URL
		if _, err := url.Parse(startURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Invalid URL '%s': %v\n", startURL, err)
			os.Exit(1)
		}
		
		maxDepth, _ := cmd.Flags().GetInt("depth")
		maxWorkers, _ := cmd.Flags().GetInt("workers")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		respectRobots, _ := cmd.Flags().GetBool("respect-robots")
		verbose, _ := cmd.Flags().GetBool("verbose")
		
		crawler := NewCrawler(maxDepth, maxWorkers, timeout, respectRobots)
		
		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), timeout*time.Duration(maxDepth*2+5))
		defer cancel()
		
		// Print header
		fmt.Printf("EchoSpider - Professional Web Crawler\n")
		fmt.Printf("Starting crawl of: %s\n", startURL)
		fmt.Printf("Configuration: depth=%d, workers=%d, timeout=%v, robots=%t\n", 
			maxDepth, maxWorkers, timeout, respectRobots)
		fmt.Println(strings.Repeat("=", 80))
		
		results := crawler.Crawl(ctx, startURL)
		
		var totalResults int
		for result := range results {
			totalResults++
			
			if result.Error != nil {
				if verbose {
					fmt.Printf("ERROR [Depth:%d] %s - %v\n", result.Depth, result.URL, result.Error)
				}
				continue
			}
			
			fmt.Printf("SUCCESS [Depth:%d] %s\n", result.Depth, result.URL)
			if result.Title != "" {
				fmt.Printf("  Title: %s\n", result.Title)
			}
			if len(result.Links) > 0 {
				fmt.Printf("  Links: %d found\n", len(result.Links))
				if verbose {
					for i, link := range result.Links {
						if i < 5 { // Show first 5 links
							fmt.Printf("    → %s\n", link)
						} else if i == 5 {
							fmt.Printf("    ... and %d more\n", len(result.Links)-5)
							break
						}
					}
				}
			}
			fmt.Println()
		}
		
		// Print final statistics
		stats := crawler.getStats()
		fmt.Println(strings.Repeat("=", 80))
		fmt.Printf("Crawl Statistics:\n")
		fmt.Printf("  Total URLs processed: %d\n", stats.TotalURLs)
		fmt.Printf("  Successful: %d\n", stats.SuccessfulURLs)
		fmt.Printf("  Failed: %d\n", stats.FailedURLs)
		fmt.Printf("  Duration: %v\n", stats.EndTime.Sub(stats.StartTime))
		if stats.TotalURLs > 0 {
			fmt.Printf("  Success rate: %.1f%%\n", float64(stats.SuccessfulURLs)/float64(stats.TotalURLs)*100)
		}
	},
}

func init() {
	rootCmd.Flags().IntP("depth", "d", 2, "Maximum crawl depth")
	rootCmd.Flags().IntP("workers", "w", 10, "Number of concurrent workers")
	rootCmd.Flags().DurationP("timeout", "t", 10*time.Second, "Request timeout")
	rootCmd.Flags().BoolP("respect-robots", "r", false, "Respect robots.txt rules")
	rootCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
