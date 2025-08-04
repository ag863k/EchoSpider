# EchoSpider

A professional concurrent web crawler built with Go.

## Features

- High-performance concurrent crawling
- Robots.txt compliance
- Advanced error handling with retry logic
- Web interface for easy operation
- Rate limiting and metadata extraction
- Production-ready deployment

## Quick Start

```bash
# Build
go build -o echospider.exe

# CLI Usage
./echospider crawl https://example.com

# Web Interface
./echospider server
# Open http://localhost:8080
```

## Command Line Options

```bash
./echospider crawl [URL] [flags]

Flags:
  -d, --depth int       Maximum crawl depth (default 2)
  -w, --workers int     Number of workers (default 10)
  -t, --timeout duration Request timeout (default 10s)
  -r, --robots          Respect robots.txt (default true)
```

## API

Start the web server and make POST requests to `/api/crawl`:

```json
{
  "url": "https://example.com",
  "maxDepth": 2,
  "maxWorkers": 10,
  "timeout": 10,
  "respectRobots": true
}
```

## Architecture

The crawler uses a concurrent worker pool pattern:

- Goroutines process URLs from a buffered job queue
- Context-based timeout and cancellation
- Thread-safe visited URL tracking with sync.Map
- Channel-based result streaming
- Cached robots.txt parsing and validation

## Performance

- Configurable worker pools (1-100 workers)
- Atomic operations for thread-safe statistics
- HTTP/2 support with connection pooling
- Automatic retry logic for transient errors
- Rate limiting to prevent server overload

## License

MIT
