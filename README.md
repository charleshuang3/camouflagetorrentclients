# Camouflage Torrent Clients

This project provides utilities to modify anacrolix/torrent client requests, making them appear as if they originate from a different torrent client software. This can be useful for bypassing certain tracker restrictions or for privacy reasons.

## Purpose

The primary goal is to "camouflage" the identity of the torrent client being used by altering the parameters sent in tracker announce requests.

## Current Features

*   **Transmission Camouflage**: Currently, the project supports modifying requests to mimic the popular [Transmission](https://transmissionbt.com/) client.

## How it Works (Conceptual)

The core logic intercepts outgoing tracker requests and replaces specific arguments (like `peer_id`, `key`, and potentially `user-agent` headers) with values typically associated with the target client (e.g., Transmission).

## Future Development

*   Support for mimicking other clients (e.g., qBittorrent, uTorrent).
*   More sophisticated camouflage techniques.

## Contributing

Contributions are welcome! Please feel free to submit pull requests or open issues.

## License

This project is licensed under the terms of the [LICENSE.txt](LICENSE.txt) file.
