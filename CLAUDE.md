# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

GoVista is a keyboard-driven wallpaper browser for [Wallhaven](https://wallhaven.cc) built with [Gio UI](https://gioui.org) (a Go GUI framework). Users browse, search, and set wallpapers entirely from the keyboard.

## Build & Run

```bash
# Build
go build .

# Run
./govista -a <api-key> -u <username>

# Release builds (cross-platform, prompts for version)
./build.sh
```

The project uses a NixOS flake for development dependencies (libGL, Wayland, Vulkan, etc.):
```bash
nix develop   # Enter dev shell with all native deps
```

No tests or linting configuration exist in this project.

## Architecture

The app is ~2100 lines across 5 files, all in `package main`:

| File | Purpose |
|------|---------|
| `main.go` | Application entry point, event loop, keyboard handling, grid layout, all modal overlays |
| `lightbox.go` | Full-resolution image preview with metadata panel, tags, color swatches |
| `thumb.go` | Thumbnail grid cell rendering, async image downloading |
| `wallhaven.go` | Wallhaven API queries, pagination, image caching to `~/.cache/govista/` |
| `config.go` | TOML config loading (`~/.config/govista/config.toml`) and CLI flag parsing |

### State & Threading

All UI state lives in a single `state` struct (`main.go:51-104`), protected by `state.mu` for cross-goroutine access. Background goroutines handle:
- Thumbnail loading (async per-cell)
- Full-resolution image loading (lightbox)
- Wallpaper metadata fetching
- Wallpaper download + setting / script invocation
- Collections fetching

The Gio event loop runs on the main thread; goroutines signal redraws via `state.window.Invalidate()`.

### Config Priority

Defaults → `config.toml` file → CLI flags (highest priority). The resolved config builds the Wallhaven query object used throughout the session.

### Key Patterns

- **Gio rendering:** Frame-by-frame layout functions; each widget is laid out fresh every frame
- **Async with fade-in:** Images load in goroutines; cells animate opacity from 0→1 on first render
- **Caching:** Full-res images are cached to disk by wallpaper ID; thumbnails are fetched from Wallhaven CDN URLs
- **Modals:** Search, collections, lightbox, and help are rendered as overlay layers on top of the grid, toggled via boolean flags on `state`
