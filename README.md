# EchoSpider

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/Build-Passing-brightgreen)](https://github.com/yourusername/echospider)

A professional concurrent web crawler built with Go that showcases advanced concurrency patterns, context handling, and robust error management.

## 🚀 Features

- **High-Performance Concurrency**: Leverages Go's goroutines and channels with lock-free data structures
- **Context-Aware Operations**: Proper timeout and cancellation support with retry logic
- **Intelligent Link Discovery**: Extracts links, images, metadata with smart URL resolution
- **Robots.txt Compliance**: Advanced robots.txt parsing with intelligent caching
- **Professional CLI**: Feature-rich interface with emoji output and detailed statistics
- **Robust Error Handling**: Comprehensive error reporting with automatic retry for transient failures
- **Thread-Safe Operations**: Lock-free concurrent operations using sync.Map and atomic counters
- **Performance Monitoring**: Real-time statistics tracking with bandwidth and timing metrics

## 🛠️ Installation

### Prerequisites
- Go 1.21 or later

### Build from Source
```bash
git clone https://github.com/yourusername/echospider.git
cd echospider
go mod tidy
go build -o echospider
```

## 📖 Usage

### Basic Usage
```bash
# Crawl a website with default settings
./echospider https://example.com

# Get help
./echospider --help
```

### Advanced Usage
```bash
# Crawl with custom depth and workers
./echospider https://example.com --depth 3 --workers 20

# Respect robots.txt with custom timeout
./echospider https://example.com --respect-robots --timeout 30s

# Full configuration
./echospider https://example.com \
    --depth 5 \
    --workers 50 \
    --timeout 15s \
    --respect-robots \
    --verbose
```

### Command Line Options

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--depth` | `-d` | `3` | Maximum crawl depth |
| `--workers` | `-w` | `20` | Number of concurrent workers |
| `--timeout` | `-t` | `30s` | Request timeout duration |
| `--respect-robots` | `-r` | `false` | Respect robots.txt rules |
| `--verbose` | `-v` | `false` | Enable verbose output |
| `--show-images` | `-i` | `false` | Show discovered images |

## 🏗️ Architecture

EchoSpider implements a sophisticated concurrent architecture:

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   URL Queue     │───▶│  Worker Pool    │───▶│  Result Stream  │
│   (Buffered)    │    │  (Goroutines)   │    │   (Channel)     │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         ▲                       │                       │
         │                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│ Link Extractor  │    │ Robots Checker  │    │ Output Handler  │
│                 │    │   (Cached)      │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

### Key Components

- **Concurrent Workers**: Goroutines process URLs from a buffered job queue
- **Context Management**: Proper timeout and cancellation propagation
- **Thread-Safe State**: Mutex-protected visited URL tracking
- **Channel Communication**: Non-blocking result streaming
- **Robots.txt Compliance**: Cached robots.txt parsing and validation

## 🧪 Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with verbose output
go test -v ./...
```

## 📊 Performance

EchoSpider is designed for high performance:

- **Concurrent Processing**: Configurable worker pools (default: 20 workers)
- **Efficient Memory Usage**: Streaming results, atomic operations, sync.Map for visited URLs
- **Smart Caching**: Robots.txt caching, URL deduplication with lock-free operations
- **Timeout Management**: Per-request timeouts, context-based cancellation
- **Retry Logic**: Automatic retry for transient network errors
- **Rate Limiting**: Built-in rate limiting to prevent server overload
- **Memory Optimization**: Lock-free concurrent data structures
- **Connection Pooling**: HTTP/2 support with optimized transport settings

## 🔧 Configuration

### Configuration File (config.json)
```json
{
  "maxDepth": 2,
  "maxWorkers": 10,
  "timeoutSec": 10,
  "respectRobots": false,
  "userAgent": "EchoSpider/1.0",
  "outputFormat": "console"
}
```

### Programmatic Usage
```go
package main

import (
    "context"
    "fmt"
    "time"
)

func main() {
    crawler := NewCrawler(2, 10, 10*time.Second, false)
    ctx := context.Background()
    
    results := crawler.Crawl(ctx, "https://example.com")
    for result := range results {
        if result.Error != nil {
            fmt.Printf("Error: %v\n", result.Error)
            continue
        }
        fmt.Printf("Crawled: %s\n", result.URL)
    }
}
```

## 🔒 Best Practices

1. **Set appropriate timeouts** to prevent hanging
2. **Use context cancellation** for graceful shutdown
3. **Monitor memory usage** for large crawls
4. **Respect robots.txt** for ethical crawling
5. **Implement rate limiting** for production use
6. **Handle errors gracefully** in result processing

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🌟 Acknowledgments

- Built with Go's excellent concurrency primitives
- CLI powered by [Cobra](https://github.com/spf13/cobra)
- HTML parsing with [golang.org/x/net/html](https://pkg.go.dev/golang.org/x/net/html)

---

**EchoSpider** - Demonstrating Go's concurrency excellence in web crawling.
