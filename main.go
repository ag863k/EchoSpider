package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/net/html"
)

const (
	UserAgent          = "EchoSpider/1.0 (+https://github.com/echospider/echospider)"
	MaxJobQueueSize    = 10000
	MaxResultQueueSize = 1000
	DefaultTimeout     = 30 * time.Second
	DefaultMaxDepth    = 3
	DefaultMaxWorkers  = 20
	MinWorkers         = 1
	MaxWorkers         = 500
	RateLimitDelay     = 100 * time.Millisecond
)

type Crawler struct {
	maxDepth      int
	maxWorkers    int
	timeout       time.Duration
	visited       sync.Map
	client        *http.Client
	respectRobots bool
	baseURL       *url.URL
	robotsChecker *RobotsChecker
	stats         *CrawlStats
	rateLimit     time.Duration
	maxRetries    int
	userAgent     string
}

type CrawlStats struct {
	TotalURLs      int64
	SuccessfulURLs int64
	FailedURLs     int64
	SkippedURLs    int64
	StartTime      time.Time
	EndTime        time.Time
	BytesDownloaded int64
	mu             sync.RWMutex
}

type CrawlResult struct {
	URL           string
	Title         string
	Description   string
	Keywords      []string
	Links         []string
	Images        []string
	Depth         int
	Error         error
	StatusCode    int
	ContentLength int64
	ResponseTime  time.Duration
	Stats         *CrawlStats
}

func NewCrawler(maxDepth, maxWorkers int, timeout time.Duration, respectRobots bool) *Crawler {
	// Validate and adjust parameters
	if maxWorkers < MinWorkers {
		maxWorkers = MinWorkers
	}
	if maxWorkers > MaxWorkers {
		maxWorkers = MaxWorkers
	}
	if maxWorkers > runtime.NumCPU()*10 {
		maxWorkers = runtime.NumCPU() * 10
	}
	if timeout < time.Second {
		timeout = DefaultTimeout
	}

	// Create optimized HTTP client
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},
	}

	return &Crawler{
		maxDepth:      maxDepth,
		maxWorkers:    maxWorkers,
		timeout:       timeout,
		respectRobots: respectRobots,
		robotsChecker: NewRobotsChecker(),
		rateLimit:     RateLimitDelay,
		maxRetries:    3,
		userAgent:     UserAgent,
		stats: &CrawlStats{
			StartTime: time.Now(),
		},
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

func (s *CrawlStats) incrementTotal() {
	atomic.AddInt64(&s.TotalURLs, 1)
}

func (s *CrawlStats) incrementSuccessful() {
	atomic.AddInt64(&s.SuccessfulURLs, 1)
}

func (s *CrawlStats) incrementFailed() {
	atomic.AddInt64(&s.FailedURLs, 1)
}

func (s *CrawlStats) incrementSkipped() {
	atomic.AddInt64(&s.SkippedURLs, 1)
}

func (s *CrawlStats) addBytes(bytes int64) {
	atomic.AddInt64(&s.BytesDownloaded, bytes)
}

func (s *CrawlStats) getSnapshot() CrawlStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return CrawlStats{
		TotalURLs:       atomic.LoadInt64(&s.TotalURLs),
		SuccessfulURLs:  atomic.LoadInt64(&s.SuccessfulURLs),
		FailedURLs:      atomic.LoadInt64(&s.FailedURLs),
		SkippedURLs:     atomic.LoadInt64(&s.SkippedURLs),
		BytesDownloaded: atomic.LoadInt64(&s.BytesDownloaded),
		StartTime:       s.StartTime,
		EndTime:         time.Now(),
	}
}

func (c *Crawler) isVisited(url string) bool {
	_, exists := c.visited.Load(url)
	return exists
}

func (c *Crawler) markVisited(url string) bool {
	_, loaded := c.visited.LoadOrStore(url, true)
	return !loaded // Returns true if this is the first time we're storing this URL
}

func (c *Crawler) crawlURL(ctx context.Context, rawURL string) (*CrawlResult, error) {
	startTime := time.Now()
	
	// Rate limiting
	time.Sleep(c.rateLimit)
	
	// Robots.txt check
	if c.respectRobots && !c.robotsChecker.CanCrawl(rawURL, c.userAgent) {
		c.stats.incrementSkipped()
		return nil, fmt.Errorf("robots.txt disallows crawling")
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		
		result, err := c.attemptCrawl(ctx, rawURL, startTime)
		if err == nil {
			c.stats.incrementSuccessful()
			if result.ContentLength > 0 {
				c.stats.addBytes(result.ContentLength)
			}
			return result, nil
		}
		
		lastErr = err
		if !c.isRetryableError(err) {
			break
		}
	}
	
	c.stats.incrementFailed()
	return nil, fmt.Errorf("failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

func (c *Crawler) attemptCrawl(ctx context.Context, rawURL string, startTime time.Time) (*CrawlResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Set comprehensive headers
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	responseTime := time.Since(startTime)
	
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return &CrawlResult{
			URL:          rawURL,
			StatusCode:   resp.StatusCode,
			ResponseTime: responseTime,
		}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(contentType), "text/html") {
		return &CrawlResult{
			URL:          rawURL,
			StatusCode:   resp.StatusCode,
			ResponseTime: responseTime,
		}, fmt.Errorf("not HTML content: %s", contentType)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return &CrawlResult{
			URL:          rawURL,
			StatusCode:   resp.StatusCode,
			ResponseTime: responseTime,
		}, fmt.Errorf("parsing HTML: %w", err)
	}

	contentLength := resp.ContentLength
	if contentLength <= 0 {
		contentLength = 0
	}

	title := c.extractTitle(doc)
	description := c.extractDescription(doc)
	keywords := c.extractKeywords(doc)
	links := c.extractLinks(doc, rawURL)
	images := c.extractImages(doc, rawURL)

	return &CrawlResult{
		URL:           rawURL,
		Title:         title,
		Description:   description,
		Keywords:      keywords,
		Links:         links,
		Images:        images,
		StatusCode:    resp.StatusCode,
		ContentLength: contentLength,
		ResponseTime:  responseTime,
		Stats:         &c.stats.getSnapshot(),
	}, nil
}

func (c *Crawler) isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	
	errStr := strings.ToLower(err.Error())
	retryableErrors := []string{
		"timeout",
		"connection reset",
		"connection refused",
		"temporary failure",
		"server closed",
		"broken pipe",
	}
	
	for _, retryable := range retryableErrors {
		if strings.Contains(errStr, retryable) {
			return true
		}
	}
	
	return false
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

func (c *Crawler) extractDescription(doc *html.Node) string {
	var description string
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			var name, content string
			for _, attr := range n.Attr {
				if attr.Key == "name" && (strings.ToLower(attr.Val) == "description" || strings.ToLower(attr.Val) == "og:description") {
					name = attr.Val
				}
				if attr.Key == "content" {
					content = attr.Val
				}
			}
			if name != "" && content != "" {
				description = strings.TrimSpace(content)
				return
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
			if description != "" {
				return
			}
		}
	}
	traverse(doc)
	return description
}

func (c *Crawler) extractKeywords(doc *html.Node) []string {
	var keywords []string
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			var name, content string
			for _, attr := range n.Attr {
				if attr.Key == "name" && strings.ToLower(attr.Val) == "keywords" {
					name = attr.Val
				}
				if attr.Key == "content" {
					content = attr.Val
				}
			}
			if name != "" && content != "" {
				keywordList := strings.Split(content, ",")
				for _, keyword := range keywordList {
					keyword = strings.TrimSpace(keyword)
					if keyword != "" {
						keywords = append(keywords, keyword)
					}
				}
				return
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
			if len(keywords) > 0 {
				return
			}
		}
	}
	traverse(doc)
	return keywords
}

func (c *Crawler) extractImages(doc *html.Node, baseURL string) []string {
	var images []string
	imageMap := make(map[string]bool)
	
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			for _, attr := range n.Attr {
				if attr.Key == "src" {
					imgURL := c.resolveURL(baseURL, attr.Val)
					if imgURL != "" && !imageMap[imgURL] {
						images = append(images, imgURL)
						imageMap[imgURL] = true
					}
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}
	traverse(doc)
	return images
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
						
						c.stats.incrementTotal()
						
						// Check if already visited and mark as visited atomically
						if !c.markVisited(job.url) {
							c.stats.incrementSkipped()
							continue
						}
						
						result, err := c.crawlURL(ctx, job.url)
						if err != nil {
							results <- CrawlResult{URL: job.url, Depth: job.depth, Error: err}
							continue
						}
						
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
		showImages, _ := cmd.Flags().GetBool("show-images")
		
		// Validate parameters
		if maxDepth < 0 {
			fmt.Fprintf(os.Stderr, "Error: depth must be >= 0\n")
			os.Exit(1)
		}
		if maxWorkers < MinWorkers || maxWorkers > MaxWorkers {
			fmt.Fprintf(os.Stderr, "Error: workers must be between %d and %d\n", MinWorkers, MaxWorkers)
			os.Exit(1)
		}
		
		crawler := NewCrawler(maxDepth, maxWorkers, timeout, respectRobots)
		
		// Create context with timeout
		totalTimeout := timeout * time.Duration(maxDepth*2+10)
		ctx, cancel := context.WithTimeout(context.Background(), totalTimeout)
		defer cancel()
		
		// Print header
		fmt.Printf("🕷️  EchoSpider - Professional Web Crawler\n")
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("Starting crawl of: %s\n", startURL)
		fmt.Printf("Configuration: depth=%d, workers=%d, timeout=%v, robots=%t\n", 
			maxDepth, maxWorkers, timeout, respectRobots)
		fmt.Printf("Runtime: Go %s on %s/%s with %d CPUs\n", 
			runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		
		results := crawler.Crawl(ctx, startURL)
		
		var totalResults int
		for result := range results {
			totalResults++
			
			if result.Error != nil {
				if verbose {
					fmt.Printf("❌ ERROR [Depth:%d] %s\n", result.Depth, result.URL)
					fmt.Printf("   └─ %v\n", result.Error)
				}
				continue
			}
			
			fmt.Printf("✅ SUCCESS [Depth:%d] %s", result.Depth, result.URL)
			if result.ResponseTime > 0 {
				fmt.Printf(" ⏱️ %v", result.ResponseTime.Round(time.Millisecond))
			}
			if result.StatusCode > 0 {
				fmt.Printf(" [%d]", result.StatusCode)
			}
			fmt.Println()
			
			if result.Title != "" {
				fmt.Printf("   📄 Title: %s\n", result.Title)
			}
			if result.Description != "" && verbose {
				fmt.Printf("   📝 Description: %.100s...\n", result.Description)
			}
			if len(result.Keywords) > 0 && verbose {
				fmt.Printf("   🏷️  Keywords: %s\n", strings.Join(result.Keywords[:min(5, len(result.Keywords))], ", "))
			}
			if len(result.Links) > 0 {
				fmt.Printf("   🔗 Links: %d found", len(result.Links))
				if verbose && len(result.Links) > 0 {
					fmt.Printf(" (showing first 3)")
					for i, link := range result.Links {
						if i < 3 {
							fmt.Printf("\n      → %s", link)
						} else {
							fmt.Printf("\n      ... and %d more", len(result.Links)-3)
							break
						}
					}
				}
				fmt.Println()
			}
			if len(result.Images) > 0 && showImages {
				fmt.Printf("   🖼️  Images: %d found\n", len(result.Images))
				if verbose {
					for i, img := range result.Images {
						if i < 3 {
							fmt.Printf("      → %s\n", img)
						} else {
							fmt.Printf("      ... and %d more\n", len(result.Images)-3)
							break
						}
					}
				}
			}
			if result.ContentLength > 0 && verbose {
				fmt.Printf("   📊 Size: %s\n", formatBytes(result.ContentLength))
			}
			fmt.Println()
		}
		
		// Print final statistics
		stats := crawler.stats.getSnapshot()
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("📊 Crawl Statistics\n")
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("Total URLs processed: %d\n", stats.TotalURLs)
		fmt.Printf("✅ Successful: %d\n", stats.SuccessfulURLs)
		fmt.Printf("❌ Failed: %d\n", stats.FailedURLs)
		fmt.Printf("⏭️  Skipped: %d\n", stats.SkippedURLs)
		if stats.BytesDownloaded > 0 {
			fmt.Printf("📥 Data downloaded: %s\n", formatBytes(stats.BytesDownloaded))
		}
		fmt.Printf("⏱️  Duration: %v\n", stats.EndTime.Sub(stats.StartTime).Round(time.Millisecond))
		if stats.TotalURLs > 0 {
			successRate := float64(stats.SuccessfulURLs) / float64(stats.TotalURLs) * 100
			fmt.Printf("📈 Success rate: %.1f%%\n", successRate)
			
			avgTime := stats.EndTime.Sub(stats.StartTime) / time.Duration(stats.TotalURLs)
			fmt.Printf("⚡ Average time per URL: %v\n", avgTime.Round(time.Millisecond))
		}
	},
}

func init() {
	rootCmd.Flags().IntP("depth", "d", DefaultMaxDepth, "Maximum crawl depth")
	rootCmd.Flags().IntP("workers", "w", DefaultMaxWorkers, "Number of concurrent workers")
	rootCmd.Flags().DurationP("timeout", "t", DefaultTimeout, "Request timeout")
	rootCmd.Flags().BoolP("respect-robots", "r", false, "Respect robots.txt rules")
	rootCmd.Flags().BoolP("verbose", "v", false, "Enable verbose output")
	rootCmd.Flags().BoolP("show-images", "i", false, "Show discovered images")
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
