# GoVista

A keyboard-driven wallpaper browser for [Wallhaven](https://wallhaven.cc), built with [Gio UI](https://gioui.org).

Browse, search, and set wallpapers without leaving your keyboard.

## Features

- Responsive thumbnail grid with smooth fade-in loading
- Lightbox view with full-resolution preview and metadata (resolution, tags, colors, uploader)
- Search, sorting modes (Hot, Toplist, Latest, Random), and category/purity filters
- Browse personal or public Wallhaven collections
- Downloads full-res wallpapers to `~/.cache/govista/` (reuses cached files)
- Sets wallpaper directly, or runs a custom script with the file path
- Prints selected wallpaper path to stdout (`-o`) for scripting
- Configurable via TOML config file and CLI flags

## Installation

```sh
go install github.com/davenicholson-xyz/govista@latest
```

Or build from source:

```sh
git clone https://github.com/davenicholson-xyz/govista
cd govista
go build .
```

### Dependencies

Requires the following libraries at build time 

```
pkg-config libGL wayland wayland-protocols libxkbcommon
libx11 libxcursor libxfixes libxcb vulkan-loader vulkan-headers
```

On NixOS a `flake.nix` is included.

## Configuration

Create `~/.config/govista/config.toml`:

```toml
api_key        = "your-wallhaven-api-key"
username       = "your-wallhaven-username"
categories     = "111"   # General | Anime | People bitmask
purity         = "100"   # SFW | Sketchy | NSFW bitmask
min-resolution = "1920x1080"
thumb-size     = 200
output         = false
script         = ""
keep-open      = false
```

CLI flags override config file values. An API key is required to access NSFW content or private collections.

## Usage

```
govista [flags]

  -a, --api-key         Wallhaven API key
  -u, --username        Wallhaven username
  -q, --query           Default search query
  -c, --categories      Category bitmask (default: 111)
  -p, --purity          Purity bitmask (default: 100)
  -r, --min-resolution  Minimum resolution, e.g. 1920x1080
  -t, --thumb-size      Thumbnail size in dp (default: 200)
  -s, --script          Script to run with the wallpaper path as argument
  -o, --output          Print selected wallpaper path to stdout
  -k, --keep-open       Keep window open after setting a wallpaper

  --hot / -H            Start with Hot sorting
  --top / -T            Start with Toplist sorting
  --latest / -l         Start with Latest sorting
  --random / -R         Start with Random sorting
```

### Purity / Category bitmasks

Each character in the 3-character bitmask enables (`1`) or disables (`0`) a flag:

| Position | Categories | Purity  |
|----------|-----------|---------|
| 1        | General   | SFW     |
| 2        | Anime     | Sketchy |
| 3        | People    | NSFW    |

Examples: `categories = "110"` (General + Anime), `purity = "110"` (SFW + Sketchy)

### Script mode

When `--script` is set, govista runs the script instead of setting the wallpaper directly:

```sh
govista --script ~/.local/bin/set-wallpaper --output
```

The script receives the cached file path as its first argument.

## Keybindings

### Grid

| Key           | Action                        |
|---------------|-------------------------------|
| `h` / `←`    | Move left                     |
| `l` / `→`    | Move right                    |
| `k` / `↑`    | Move up                       |
| `j` / `↓`    | Move down                     |
| `Enter`       | Set selected wallpaper        |
| `p`           | Open lightbox for selected    |
| `o`           | Open in browser               |
| `S` / `/`     | Open search                   |
| `C`           | Open collections              |
| `H`           | Sort: Hot                     |
| `T`           | Sort: Toplist                 |
| `L`           | Sort: Latest                  |
| `R`           | Sort: Random                  |
| `q` / `Esc`  | Quit                          |

### Lightbox

| Key           | Action                        |
|---------------|-------------------------------|
| `Enter`       | Set wallpaper                 |
| `o`           | Open in browser               |
| `h` / `l`    | Navigate tags                 |
| `Esc` / `q`  | Close lightbox                |

### Search / Collections modal

| Key           | Action                        |
|---------------|-------------------------------|
| `Enter`       | Confirm                       |
| `Esc`         | Cancel / close                |

## License

MIT
