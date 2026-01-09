# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SMS Gateway daemon that accepts HTTP POST and MQTT requests to send SMS messages via an AT-compatible modem. Single-file Go application with Debian packaging for production deployment.

## Architecture

### Core Components

**Modem Layer** (sms-gw.go:115-231)
- `Modem` struct manages serial port communication with AT-compatible modem
- Initialization sequence: AT handshake → CPIN check/PIN unlock → CMGF=1 (text mode) → CSCS=GSM (character set)
- Thread-safe with mutex-protected send operations
- `SendSMS()` expects prompt ('>') after AT+CMGS command, then sends message body + Ctrl+Z

**Gateway & Job Queue** (sms-gw.go:255-308)
- `Gateway` coordinates modem access, rate limiting, and job queue
- Buffered channel (1024) holds jobs with retry state
- Single worker goroutine processes queue sequentially
- Auto-generated job IDs (SHA1 of to/message/timestamp) if not provided

**Rate Limiting** (sms-gw.go:233-253)
- Sliding window implementation tracking send times
- Configurable messages per minute (default: 30)
- Jobs exceeding limit backoff 2s and re-queue

**Retry Logic** (sms-gw.go:292-304)
- Failed sends retry with exponential backoff (800-1400ms jitter)
- Configurable max retries (default: 3)
- Permanent failures logged but don't crash daemon

### Interface Adapters

**HTTP Server** (sms-gw.go:310-342)
- POST /sms with JSON payload: `{"to": "+...", "message": "...", "id": "optional"}`
- Optional Bearer token authentication via `HTTP_TOKEN` env var
- GET /healthz for monitoring
- Graceful shutdown on SIGINT/SIGTERM

**MQTT Client** (sms-gw.go:344-373)
- Subscribes to configurable topic (default: `sms/send`)
- Same JSON payload format as HTTP
- Auto-reconnect on connection loss
- QoS 0 (fire and forget)

## Build & Development

### Building

```bash
# Standard build
go build -o sms-gw sms-gw.go

# With dependencies
go mod download
go build -o sms-gw sms-gw.go
```

### Debian Package

```bash
# Build .deb package
dpkg-buildpackage -us -uc -b

# Clean build artifacts
debian/rules clean
```

The Debian package:
- Installs binary to `/usr/bin/sms-gw`
- Creates systemd service `sms-gw.service`
- Adds `smsgw` user in `dialout` group for serial port access
- Config in `/etc/default/sms-gw` (environment variables)

## Configuration

All config via environment variables (see sms-gw.go:13-26 for complete list):

**Required:**
- `PORT` - Serial device path (default: /dev/ttyUSB2)
- `BAUD` - Baud rate (default: 115200)

**Optional:**
- `SIM_PIN` - If SIM requires PIN unlock
- `HTTP_ADDR` - HTTP listen address (default: :8080, empty disables)
- `HTTP_TOKEN` - Bearer auth token for HTTP API
- `MQTT_BROKER` - MQTT broker URL (empty disables)
- `MQTT_CLIENT_ID`, `MQTT_TOPIC`, `MQTT_USERNAME`, `MQTT_PASSWORD`
- `RATE_PER_MIN` - Rate limit (default: 30)
- `MAX_RETRIES` - Retry attempts (default: 3)
- `ECHO` - Set to "1" for ATE1 (modem echo on), else ATE0

## Testing & Debugging

### Manual Testing

```bash
# Test HTTP endpoint
curl -X POST http://localhost:8080/sms \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"to": "+1234567890", "message": "test"}'

# Test MQTT (requires mosquitto-clients)
mosquitto_pub -h localhost -t sms/send \
  -m '{"to": "+1234567890", "message": "test"}'

# Check service status
systemctl status sms-gw
journalctl -u sms-gw -f
```

### Serial Port Testing

```bash
# Verify modem is accessible
ls -l /dev/ttyUSB*

# Test AT commands manually (requires minicom/screen)
screen /dev/ttyUSB2 115200
# Type: AT (should respond OK)
# Type: AT+CPIN? (check SIM status)
```

## Code Patterns

### Adding New Configuration

1. Add env var to `Config` struct (sms-gw.go:64-78)
2. Add parsing in `loadConfig()` (sms-gw.go:94-112)
3. Update debian/sms-gw.default with commented example

### Modem Communication

- All modem ops must acquire `m.mu` lock
- Use `m.send(cmd, expectPrompt, timeout)` for AT commands
- `readLines()` blocks until deadline - adjust timeouts for slow modems
- Check for both "OK" and specific response patterns (e.g., "+CMGS:")

### Adding New Endpoints

HTTP handlers in `startHTTP()` run in goroutines - use `g.Enqueue()` to hand off work to the single-threaded worker. Never call `g.m.SendSMS()` directly from handlers (bypasses rate limiting and breaks concurrency safety).
