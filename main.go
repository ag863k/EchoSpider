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

type Crawler struct {
	maxDepth     int
	maxWorkers   int
	timeout      time.Duration
	visited      map[string]bool
	visitedMu    sync.RWMutex
	client       *http.Client
	respectRobots bool
	baseURL      *url.URL
	robotsChecker *RobotsChecker
}

type CrawlResult struct {
	URL    string
	Title  string
	Links  []string
	Depth  int
	Error  error
}

func NewCrawler(maxDepth, maxWorkers int, timeout time.Duration, respectRobots bool) *Crawler {
	return &Crawler{
		maxDepth:     maxDepth,
		maxWorkers:   maxWorkers,
		timeout:      timeout,
		visited:      make(map[string]bool),
		respectRobots: respectRobots,
		robotsChecker: NewRobotsChecker(),
		client: &http.Client{
			Timeout: timeout,
		},
	}
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
	if c.respectRobots && !c.robotsChecker.CanCrawl(rawURL, "EchoSpider/1.0") {
		return nil, fmt.Errorf("robots.txt disallows crawling")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "EchoSpider/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	title := c.extractTitle(doc)
	links := c.extractLinks(doc, rawURL)

	return &CrawlResult{
		URL:   rawURL,
		Title: title,
		Links: links,
	}, nil
}

func (c *Crawler) extractTitle(doc *html.Node) string {
	var title string
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "title" {
			if n.FirstChild != nil {
				title = strings.TrimSpace(n.FirstChild.Data)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			traverse(child)
		}
	}
	traverse(doc)
	return title
}

func (c *Crawler) extractLinks(doc *html.Node, baseURL string) []string {
	var links []string
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					link := c.resolveURL(baseURL, attr.Val)
					if link != "" && c.isInternalLink(link) {
						links = append(links, link)
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
	base, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	
	link, err := url.Parse(href)
	if err != nil {
		return ""
	}
	
	resolved := base.ResolveReference(link)
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
	results := make(chan CrawlResult, 100)
	
	baseURL, err := url.Parse(startURL)
	if err != nil {
		go func() {
			defer close(results)
			results <- CrawlResult{URL: startURL, Error: err}
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
		
		jobs := make(chan job, 100)
		var wg sync.WaitGroup
		
		for i := 0; i < c.maxWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobs {
					if c.isVisited(job.url) {
						continue
					}
					c.markVisited(job.url)
					
					result, err := c.crawlURL(ctx, job.url)
					if err != nil {
						results <- CrawlResult{URL: job.url, Depth: job.depth, Error: err}
						continue
					}
					
					result.Depth = job.depth
					results <- *result
					
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
				}
			}()
		}
		
		jobs <- job{url: startURL, depth: 0}
		
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
	Short: "A concurrent web crawler built with Go",
	Long:  "EchoSpider is a professional concurrent web crawler that extracts and follows internal links.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		startURL := args[0]
		
		maxDepth, _ := cmd.Flags().GetInt("depth")
		maxWorkers, _ := cmd.Flags().GetInt("workers")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		respectRobots, _ := cmd.Flags().GetBool("respect-robots")
		
		crawler := NewCrawler(maxDepth, maxWorkers, timeout, respectRobots)
		
		ctx, cancel := context.WithTimeout(context.Background(), timeout*time.Duration(maxDepth+1))
		defer cancel()
		
		fmt.Printf("Starting crawl of %s\n", startURL)
		fmt.Printf("Max depth: %d, Workers: %d, Timeout: %v\n", maxDepth, maxWorkers, timeout)
		fmt.Println(strings.Repeat("-", 60))
		
		results := crawler.Crawl(ctx, startURL)
		
		for result := range results {
			if result.Error != nil {
				fmt.Printf("ERROR [%s]: %v\n", result.URL, result.Error)
				continue
			}
			
			fmt.Printf("CRAWLED [Depth: %d] %s\n", result.Depth, result.URL)
			if result.Title != "" {
				fmt.Printf("  Title: %s\n", result.Title)
			}
			if len(result.Links) > 0 {
				fmt.Printf("  Links found: %d\n", len(result.Links))
			}
			fmt.Println()
		}
	},
}

func init() {
	rootCmd.Flags().IntP("depth", "d", 2, "Maximum crawl depth")
	rootCmd.Flags().IntP("workers", "w", 10, "Number of concurrent workers")
	rootCmd.Flags().DurationP("timeout", "t", 10*time.Second, "Request timeout")
	rootCmd.Flags().BoolP("respect-robots", "r", false, "Respect robots.txt")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}
