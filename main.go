package main

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/net/html"
)

const (
	UserAgent          = "EchoSpider/1.0"
	MaxJobQueueSize    = 10000
	MaxResultQueueSize = 1000
	DefaultTimeout     = 10 * time.Second
	DefaultMaxDepth    = 2
	DefaultMaxWorkers  = 10
	MinWorkers         = 1
	MaxWorkers         = 100
	RateLimitDelay     = 50 * time.Millisecond
	MaxBodySize        = 10 * 1024 * 1024
	Version            = "1.0.0"
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
	urlsCrawled      int64
	urlsSkipped      int64
	urlsSuccessful   int64
	urlsFailed       int64
	bytesTransferred int64
	StartTime        time.Time
	mu               sync.RWMutex
}

func (s *CrawlStats) incrementTotal() {
	atomic.AddInt64(&s.urlsCrawled, 1)
}

func (s *CrawlStats) incrementSkipped() {
	atomic.AddInt64(&s.urlsSkipped, 1)
}

func (s *CrawlStats) incrementSuccessful() {
	atomic.AddInt64(&s.urlsSuccessful, 1)
}

func (s *CrawlStats) incrementFailed() {
	atomic.AddInt64(&s.urlsFailed, 1)
}

func (s *CrawlStats) addBytes(bytes int64) {
	atomic.AddInt64(&s.bytesTransferred, bytes)
}

type CrawlResult struct {
	URL           string    `json:"url"`
	Title         string    `json:"title,omitempty"`
	Description   string    `json:"description,omitempty"`
	Keywords      []string  `json:"keywords,omitempty"`
	Links         []Link    `json:"links,omitempty"`
	Images        []Image   `json:"images,omitempty"`
	Depth         int       `json:"depth"`
	Error         error     `json:"error,omitempty"`
	StatusCode    int       `json:"statusCode"`
	ContentType   string    `json:"contentType,omitempty"`
	ResponseTime  time.Duration `json:"responseTime,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

type Link struct {
	URL  string `json:"url"`
	Text string `json:"text,omitempty"`
}

type Image struct {
	URL string `json:"url"`
	Alt string `json:"alt,omitempty"`
}

type Stats struct {
	TotalPages  int64         `json:"totalPages"`
	TotalLinks  int64         `json:"totalLinks"`
	TotalImages int64         `json:"totalImages"`
	Duration    time.Duration `json:"duration"`
}

func NewCrawler(maxDepth, maxWorkers int, timeout time.Duration, respectRobots bool) *Crawler {
	if maxWorkers < MinWorkers {
		maxWorkers = MinWorkers
	}
	if maxWorkers > MaxWorkers {
		maxWorkers = MaxWorkers
	}
	if timeout < time.Second {
		timeout = DefaultTimeout
	}

	transport := &http.Transport{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     60 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
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
		},
	}
}

func (s *CrawlStats) incrementTotal() {
	atomic.AddInt64(&s.urlsCrawled, 1)
}

func (s *CrawlStats) incrementSkipped() {
	atomic.AddInt64(&s.urlsSkipped, 1)
}

func (s *CrawlStats) incrementSuccessful() {
	atomic.AddInt64(&s.urlsSuccessful, 1)
}

func (s *CrawlStats) incrementFailed() {
	atomic.AddInt64(&s.urlsFailed, 1)
}

func (s *CrawlStats) addBytes(bytes int64) {
	atomic.AddInt64(&s.bytesTransferred, bytes)
}

func (s *CrawlStats) getSnapshot() CrawlStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return CrawlStats{
		urlsCrawled:      atomic.LoadInt64(&s.urlsCrawled),
		urlsSuccessful:   atomic.LoadInt64(&s.urlsSuccessful),
		urlsFailed:       atomic.LoadInt64(&s.urlsFailed),
		urlsSkipped:      atomic.LoadInt64(&s.urlsSkipped),
		bytesTransferred: atomic.LoadInt64(&s.bytesTransferred),
		StartTime:        s.StartTime,
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
	
	time.Sleep(c.rateLimit)
	
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

	bodyReader := resp.Body
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return &CrawlResult{
				URL:          rawURL,
				StatusCode:   resp.StatusCode,
				ResponseTime: responseTime,
			}, fmt.Errorf("creating gzip reader: %w", err)
		}
		defer gzipReader.Close()
		bodyReader = gzipReader
	}

	doc, err := html.Parse(bodyReader)
	if err != nil {
		return &CrawlResult{
			URL:          rawURL,
			StatusCode:   resp.StatusCode,
			ResponseTime: responseTime,
		}, fmt.Errorf("parsing HTML: %w", err)
	}

	title := c.extractTitle(doc)
	description := c.extractDescription(doc)
	keywords := c.extractKeywords(doc)
	links := c.extractLinks(doc, parsedURL)
	images := c.extractImages(doc, parsedURL)

	return &CrawlResult{
		URL:           rawURL,
		Title:         title,
		Description:   description,
		Keywords:      keywords,
		Links:         links,
		Images:        images,
		StatusCode:    resp.StatusCode,
		ContentType:   contentType,
		ResponseTime:  responseTime,
		Timestamp:     time.Now(),
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

func (c *Crawler) extractImages(doc *html.Node, baseURL *url.URL) []Image {
	var images []Image
	imageMap := make(map[string]bool)
	
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			var src, alt string
			for _, attr := range n.Attr {
				if attr.Key == "src" {
					src = attr.Val
				} else if attr.Key == "alt" {
					alt = attr.Val
				}
			}
			if src != "" {
				resolved := c.resolveURL(baseURL.String(), src)
				if resolved != "" && !imageMap[resolved] {
					images = append(images, Image{URL: resolved, Alt: alt})
					imageMap[resolved] = true
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
	return images
}

func (c *Crawler) extractLinks(doc *html.Node, baseURL *url.URL) []Link {
	var links []Link
	linkMap := make(map[string]bool)
	
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			var href, text string
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					href = attr.Val
				}
			}
			if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
				text = strings.TrimSpace(n.FirstChild.Data)
			}
			
			if href != "" {
				resolved := c.resolveURL(baseURL.String(), href)
				if resolved != "" && c.isInternalLink(resolved) && !linkMap[resolved] {
					links = append(links, Link{URL: resolved, Text: text})
					linkMap[resolved] = true
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
						
						if job.depth < c.maxDepth {
							for _, link := range result.Links {
								if !c.isVisited(link.URL) {
									select {
									case jobs <- job{url: link.URL, depth: job.depth + 1}:
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
	Use:   "echospider",
	Short: "Professional concurrent web crawler",
	Long:  "EchoSpider - High-performance concurrent web crawler built with Go",
}

var crawlCmd = &cobra.Command{
	Use:   "crawl [URL]",
	Short: "Crawl a website",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		startURL := args[0]
		
		if _, err := url.Parse(startURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Invalid URL: %v\n", err)
			os.Exit(1)
		}
		
		maxDepth, _ := cmd.Flags().GetInt("depth")
		maxWorkers, _ := cmd.Flags().GetInt("workers")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		respectRobots, _ := cmd.Flags().GetBool("robots")
		
		crawler := NewCrawler(maxDepth, maxWorkers, timeout, respectRobots)
		
		ctx, cancel := context.WithTimeout(context.Background(), timeout*10)
		defer cancel()
		
		fmt.Printf("Crawling %s (depth: %d, workers: %d)\n", startURL, maxDepth, maxWorkers)
		
		resultChan := crawler.Crawl(ctx, startURL)
		var results []CrawlResult
		
		for result := range resultChan {
			results = append(results, result)
			if result.Error != nil {
				fmt.Printf("Error %s: %v\n", result.URL, result.Error)
			} else {
				fmt.Printf("Success %s [%d] - %s\n", result.URL, result.StatusCode, result.Title)
			}
		}
		
		fmt.Printf("\nCrawl completed: %d pages found\n", len(results))
	},
}

func init() {
	crawlCmd.Flags().IntP("depth", "d", DefaultMaxDepth, "Maximum crawl depth")
	crawlCmd.Flags().IntP("workers", "w", DefaultMaxWorkers, "Number of workers")
	crawlCmd.Flags().DurationP("timeout", "t", DefaultTimeout, "Request timeout")
	crawlCmd.Flags().BoolP("robots", "r", true, "Respect robots.txt")
	rootCmd.AddCommand(crawlCmd)
}

type CrawlRequest struct {
	URL           string `json:"url"`
	MaxDepth      int    `json:"maxDepth"`
	MaxWorkers    int    `json:"maxWorkers"`
	Timeout       int    `json:"timeout"`
	RespectRobots bool   `json:"respectRobots"`
}

func serveFrontend() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		
		indexPath := filepath.Join("web", "index.html")
		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			http.Error(w, "Frontend not found", http.StatusNotFound)
			return
		}
		
		http.ServeFile(w, r, indexPath)
	})

	http.HandleFunc("/api/crawl", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req CrawlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.URL == "" {
			http.Error(w, "URL is required", http.StatusBadRequest)
			return
		}

		if req.MaxDepth <= 0 || req.MaxDepth > 10 {
			req.MaxDepth = 2
		}
		if req.MaxWorkers <= 0 || req.MaxWorkers > 100 {
			req.MaxWorkers = 10
		}
		if req.Timeout <= 0 || req.Timeout > 60 {
			req.Timeout = 10
		}

		crawler := NewCrawler(req.MaxDepth, req.MaxWorkers, time.Duration(req.Timeout)*time.Second, req.RespectRobots)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		resultChan := crawler.Crawl(ctx, req.URL)
		var results []CrawlResult

		for result := range resultChan {
			results = append(results, result)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})

	fmt.Println("Server starting on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

func init() {
	var serverCmd = &cobra.Command{
		Use:   "server",
		Short: "Start web server with frontend interface",
		Run: func(cmd *cobra.Command, args []string) {
			serveFrontend()
		},
	}
	rootCmd.AddCommand(serverCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
