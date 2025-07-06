# EchoSpider

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/Build-Passing-brightgreen)](https://github.com/yourusername/echospider)

A professional concurrent web crawler built with Go that showcases advanced concurrency patterns, context handling, and robust error management.

## 🚀 Features

- **High-Performance Concurrency**: Leverages Go's goroutines and channels for efficient concurrent crawling
- **Context-Aware Operations**: Proper timeout and cancellation support using Go's context package
- **Intelligent Link Discovery**: Extracts and follows internal links with smart URL resolution
- **Robots.txt Compliance**: Optional respect for robots.txt rules with intelligent caching
- **Professional CLI**: Feature-rich command-line interface built with Cobra
- **Robust Error Handling**: Comprehensive error reporting and graceful failure handling
- **Thread-Safe Operations**: Concurrent-safe visited URL tracking and state management

## 🛠️ Installation

### Prerequisites
- Go 1.21 or later

### Build from Source
```bash
git clone https://github.com/yourusername/echospider.git
cd echospider
go build -o echospider
```

### Install via Go
```bash
go install github.com/yourusername/echospider@latest
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
    --respect-robots
```

### Command Line Options

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--depth` | `-d` | `2` | Maximum crawl depth |
| `--workers` | `-w` | `10` | Number of concurrent workers |
| `--timeout` | `-t` | `10s` | Request timeout duration |
| `--respect-robots` | `-r` | `false` | Respect robots.txt rules |

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

- **Concurrent Processing**: Configurable worker pools (default: 10 workers)
- **Efficient Memory Usage**: Streaming results, minimal memory footprint
- **Smart Caching**: Robots.txt caching, visited URL deduplication
- **Timeout Management**: Prevents resource leaks and hanging connections

## 🔧 Configuration

### Configuration File (config.json)
```json
{
  "maxDepth": 3,
  "maxWorkers": 15,
  "timeoutSec": 15,
  "respectRobots": true,
  "userAgent": "EchoSpider/1.0",
  "outputFormat": "console"
}
```

## 🤝 Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🌟 Acknowledgments

- Built with Go's excellent concurrency primitives
- CLI powered by [Cobra](https://github.com/spf13/cobra)
- HTML parsing with [golang.org/x/net/html](https://pkg.go.dev/golang.org/x/net/html)

---

**EchoSpider** - Demonstrating Go's concurrency excellence in web crawling.
