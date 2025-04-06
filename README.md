# Camouflage Torrent Clients

This project provides utilities to modify anacrolix/torrent client requests, making them appear as if they originate from a different torrent client software. This can be useful for bypassing certain tracker restrictions or for privacy reasons.

## Usage

```go
cfg := torrent.NewDefaultClientConfig()

tr := transmission.New()
cfg.HttpRequestDirector = tr.HttpRequestDirector

c, err := torrent.NewClient(cfg)
```

## Purpose

The primary goal is to "camouflage" the identity of the torrent client being used by altering the parameters sent in tracker announce requests.

## Current Features

*   **Transmission Camouflage**: Currently, the project supports modifying requests to mimic the popular [Transmission](https://transmissionbt.com/) client.

## How it Works (Conceptual)

The core logic intercepts outgoing tracker requests and replaces specific arguments (like `peer_id`, `key`, and potentially `user-agent` headers) with values typically associated with the target client (e.g., Transmission).

## Future Development

*   Support for mimicking other clients (e.g., qBittorrent, uTorrent).
*   More sophisticated camouflage techniques.

## Related codepath

- [anacrolix/torrent/tracker/tracker.go](https://github.com/anacrolix/torrent/blob/3a656a26676c23ee845dcc5b810e1f7f06005b06/tracker/tracker.go#L59)
- [anacrolix/torrent/tracker/http/http.go](https://github.com/anacrolix/torrent/blob/3a656a26676c23ee845dcc5b810e1f7f06005b06/tracker/http/http.go#L88)
- [anacrolix/torrent/tracker_scraper.go](https://github.com/anacrolix/torrent/blob/3a656a26676c23ee845dcc5b810e1f7f06005b06/tracker_scraper.go#L127)

## Contributing

Contributions are welcome! Please feel free to submit pull requests or open issues.

## License

This project is licensed under [Apache 2.0](LICENSE.txt).
