package main

import (
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

// Thumb represents a single wallpaper thumbnail in the grid.
type Thumb struct {
	ID       string
	ThumbURL string
	FullURL  string
	cfg      Config // set by loadNextPage so the click handler can use config

	// Protected by mu; written from loader goroutine, read from render goroutine.
	mu       sync.Mutex
	img      image.Image
	loaded   bool
	loadedAt time.Time

	// Render-thread only — no mutex needed.
	imgOp      paint.ImageOp
	imgOpReady bool

	click widget.Clickable
}

// load fetches the thumbnail image in a background goroutine and signals the
// window to redraw when done.
func (t *Thumb) load(w *app.Window) {
	resp, err := http.Get(t.ThumbURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return
	}

	t.mu.Lock()
	t.img = img
	t.loaded = true
	t.loadedAt = time.Now()
	t.mu.Unlock()

	w.Invalidate()
}

// layout renders the thumbnail cell, handles clicks, and shows selection state.
func (t *Thumb) layout(gtx layout.Context, w *app.Window, selected bool) layout.Dimensions {
	// Clicked must be called BEFORE Layout: Layout drains the gesture event
	// queue internally, so any Clicked check afterwards always returns false.
	if t.click.Clicked(gtx) {
		id, url, cfg := t.ID, t.FullURL, t.cfg
		go func() {
			if err := downloadAndSet(id, url, cfg, w); err != nil {
				log.Println("govista: set wallpaper:", err)
			}
		}()
	}

	// 16:9 cell.
	cw := gtx.Constraints.Max.X
	ch := cw * 9 / 16
	gtx.Constraints = layout.Exact(image.Pt(cw, ch))

	return t.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return t.draw(gtx, selected)
	})
}

var placeholderColor = color.NRGBA{R: 40, G: 40, B: 40, A: 255}

// draw paints the thumbnail: placeholder → image (with fade-in) → borders.
func (t *Thumb) draw(gtx layout.Context, selected bool) layout.Dimensions {
	sz := gtx.Constraints.Min // Min == Max from layout.Exact above.

	// 1. Placeholder background.
	paint.FillShape(gtx.Ops, placeholderColor, clip.Rect{Max: sz}.Op())

	t.mu.Lock()
	loaded := t.loaded
	img := t.img
	loadedAt := t.loadedAt
	t.mu.Unlock()

	if loaded {
		// 2. Lazily create ImageOp on the render goroutine.
		if !t.imgOpReady {
			t.imgOp = paint.NewImageOp(img)
			t.imgOpReady = true
		}

		// 3. Draw the image scaled to cover the cell.
		wImg := widget.Image{
			Src:      t.imgOp,
			Fit:      widget.Cover,
			Position: layout.Center,
		}
		wImg.Layout(gtx)

		// 4. Fade-in overlay: starts fully opaque and fades to transparent over 300 ms.
		elapsed := time.Since(loadedAt).Seconds() / 0.3
		if elapsed < 1.0 {
			alpha := uint8((1.0 - elapsed) * 255)
			paint.FillShape(gtx.Ops,
				color.NRGBA{R: 40, G: 40, B: 40, A: alpha},
				clip.Rect{Max: sz}.Op(),
			)
			gtx.Execute(op.InvalidateCmd{})
		}
	}

	// 5. Selected border (accent colour, thicker) — drawn before hover so
	//    hover can overlay it when the user mouses over a selected cell.
	if selected {
		b := gtx.Dp(unit.Dp(3))
		bc := accentColor
		paint.FillShape(gtx.Ops, bc, clip.Rect{Max: image.Pt(sz.X, b)}.Op())
		paint.FillShape(gtx.Ops, bc, clip.Rect{Min: image.Pt(0, sz.Y-b), Max: sz}.Op())
		paint.FillShape(gtx.Ops, bc, clip.Rect{Max: image.Pt(b, sz.Y)}.Op())
		paint.FillShape(gtx.Ops, bc, clip.Rect{Min: image.Pt(sz.X-b, 0), Max: sz}.Op())
	}

	// 6. Hover border (white, semi-transparent).
	if t.click.Hovered() {
		b := gtx.Dp(unit.Dp(3))
		bc := color.NRGBA{R: 255, G: 255, B: 255, A: 220}
		paint.FillShape(gtx.Ops, bc, clip.Rect{Max: image.Pt(sz.X, b)}.Op())
		paint.FillShape(gtx.Ops, bc, clip.Rect{Min: image.Pt(0, sz.Y-b), Max: sz}.Op())
		paint.FillShape(gtx.Ops, bc, clip.Rect{Max: image.Pt(b, sz.Y)}.Op())
		paint.FillShape(gtx.Ops, bc, clip.Rect{Min: image.Pt(sz.X-b, 0), Max: sz}.Op())
	}

	return layout.Dimensions{Size: sz}
}
