# Frog Cast

A desktop application for streaming media to Chromecast devices using Wails and Go.

## Features

- Discover Chromecast devices on your network
- Stream local media files to Chromecast
- Manage playback with intuitive controls
- Support for HLS streaming (both automatic and manual modes)
- File explorer for easy media selection
- Real-time device status monitoring

## Prerequisites

- Go 1.21 or higher
- Node.js 16 or higher
- npm or yarn
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

## Installation

1. Clone the repository (with submodules):
```bash
git clone --recursive https://github.com/SomeoneIsWorking/frog-cast.git
cd frog-cast
```

2. Install dependencies:
```bash
wails build
```

## Development

To run in live development mode with hot reload:

```bash
wails dev
```

This will start:
- A Vite development server for frontend hot reload
- The Go backend with live compilation
- A dev server on http://localhost:34115 for debugging

## Building

To build a production-ready application:

```bash
wails build
```

The built application will be located in the `build/bin/` directory.

## Running headless as a user service

On Linux, Frog Cast can run with no window as a casting instance that phones
and other desktop instances drive over the network:

```bash
frog-cast serve
```

`serve` starts the remote API (on the settings file's port, default 9999,
whether or not the API is enabled there), the media server on 8888, mDNS
advertising and the cast receiver, with history, cache and downloads as in the
desktop app. It never opens a window or a display connection, so it runs
without a graphical session. Pages that need a browser engine to find their
stream are refused with the needs-extraction error, and the phone or desktop
app extracts them and sends the result. The stream address given to the TV is
worked out per cast from the route to it, so a service started at boot before
the network is up still casts.

To build it, install it to `~/.local/bin/frog-cast` and run it as the systemd
user unit `packaging/systemd/frog-cast.service`:

```bash
uv run --frozen python tools/install_service.py
```

It needs Go, npm, pkg-config and the WebKitGTK 4.1, GTK 3 and libsoup 3
development packages; it builds with `go build` directly, not the Wails CLI.
Rerun it after pulling to update; it will not restart an instance that is
casting unless given `--force-restart`. Logs go to the journal:

```bash
systemctl --user status frog-cast
journalctl --user -u frog-cast -f
```

A user service runs only while its user is logged in unless lingering is on
(`loginctl enable-linger $USER`), which starts it at boot. The firewall must
let in TCP 9999 and 8888 and UDP 5353 (mDNS).

## Project Structure

- `app.go` - Main application entry point
- `chromecast.go` - Chromecast device management
- `discovery.go` - Network device discovery
- `server.go` - HTTP server for media streaming
- `hls_*.go` - HLS streaming implementations
- `frontend/` - Vue.js frontend application
- `go-chromecast/` - Go Chromecast library

## Configuration

Edit `wails.json` to configure project settings. For more information, see the [Wails documentation](https://wails.io/docs/reference/project-config).

## License

MIT License - see LICENSE file for details

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

Built with AI assistance (Claude and other LLMs).
