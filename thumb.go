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
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Thumb represents a single wallpaper thumbnail in the grid.
type Thumb struct {
	ID       string
	ThumbURL string
	FullURL  string
	cfg      Config          // set by loadNextPage so the click handler can use config
	theme    *material.Theme // set by loadNextPage for spinner rendering

	// Protected by mu; written from loader goroutine, read from render goroutine.
	mu          sync.Mutex
	img         image.Image
	loaded      bool
	loadedAt    time.Time
	downloading bool

	// Render-thread only — no mutex needed.
	imgOp      paint.ImageOp
	imgOpReady bool

	click      widget.Clickable
	rightTag   struct{} // tag for right-click pointer events
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

// startDownload marks the thumb as downloading, kicks off the goroutine to
// fetch and set the wallpaper, and clears the flag when done.
func (t *Thumb) startDownload(w *app.Window) {
	t.mu.Lock()
	t.downloading = true
	t.mu.Unlock()
	w.Invalidate()
	id, url, cfg := t.ID, t.FullURL, t.cfg
	go func() {
		if err := downloadAndSet(id, url, cfg, w); err != nil {
			log.Println("govista: set wallpaper:", err)
		}
		t.mu.Lock()
		t.downloading = false
		t.mu.Unlock()
		w.Invalidate()
	}()
}

// startDownloadNoClose is like startDownload but always keeps the app open,
// regardless of the CloseOnSelect config setting.
func (t *Thumb) startDownloadNoClose(w *app.Window) {
	t.mu.Lock()
	t.downloading = true
	t.mu.Unlock()
	w.Invalidate()
	id, url, cfg := t.ID, t.FullURL, t.cfg
	cfg.CloseOnSelect = false
	go func() {
		if err := downloadAndSet(id, url, cfg, w); err != nil {
			log.Println("govista: set wallpaper:", err)
		}
		t.mu.Lock()
		t.downloading = false
		t.mu.Unlock()
		w.Invalidate()
	}()
}

// RightClicked returns true if a right-click (secondary button press) was
// received on this thumbnail this frame. Must be called before layout.
func (t *Thumb) RightClicked(gtx layout.Context) bool {
	clicked := false
	for {
		ev, ok := gtx.Source.Event(pointer.Filter{
			Target: &t.rightTag,
			Kinds:  pointer.Press,
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		if e.Buttons.Contain(pointer.ButtonSecondary) {
			clicked = true
		}
	}
	return clicked
}

// layout renders the thumbnail cell, handles clicks, and shows selection state.
func (t *Thumb) layout(gtx layout.Context, w *app.Window, selected bool) layout.Dimensions {
	// Clicked must be called BEFORE Layout: Layout drains the gesture event
	// queue internally, so any Clicked check afterwards always returns false.
	if t.click.Clicked(gtx) {
		t.startDownload(w)
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

	r := gtx.Dp(unit.Dp(8))
	outerRR := clip.RRect{Rect: image.Rectangle{Max: sz}, NE: r, NW: r, SE: r, SW: r}

	// Border: fill the outer rounded rect first; content drawn on top (inset)
	// leaves just the ring visible — both outer and inner edges stay rounded.
	bw := 0
	if selected || t.click.Hovered() {
		bw = gtx.Dp(unit.Dp(3))
		if selected {
			paint.FillShape(gtx.Ops, accentColor, outerRR.Op(gtx.Ops))
		}
		if t.click.Hovered() {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 220}, outerRR.Op(gtx.Ops))
		}
	}

	// Clip all content to the (possibly inset) inner rounded rect.
	cr := r - bw
	if cr < 0 {
		cr = 0
	}
	defer clip.RRect{
		Rect: image.Rectangle{Min: image.Pt(bw, bw), Max: image.Pt(sz.X-bw, sz.Y-bw)},
		NE: cr, NW: cr, SE: cr, SW: cr,
	}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, &t.rightTag)

	// 1. Placeholder background.
	paint.FillShape(gtx.Ops, placeholderColor, clip.Rect{Max: sz}.Op())

	t.mu.Lock()
	loaded := t.loaded
	img := t.img
	loadedAt := t.loadedAt
	downloading := t.downloading
	t.mu.Unlock()

	if loaded {
		// 2. Lazily create ImageOp on the render goroutine.
		if !t.imgOpReady {
			t.imgOp = paint.NewImageOp(img)
			t.imgOpReady = true
		}

		// 3. Draw the image scaled to cover the cell.
		widget.Image{
			Src:      t.imgOp,
			Fit:      widget.Cover,
			Position: layout.Center,
		}.Layout(gtx)

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

	// Spinner overlay while the full-res wallpaper is downloading.
	if downloading && t.theme != nil {
		paint.FillShape(gtx.Ops, color.NRGBA{A: 120}, clip.Rect{Max: sz}.Op())
		spinSize := gtx.Dp(unit.Dp(36))
		off := op.Offset(image.Pt((sz.X-spinSize)/2, (sz.Y-spinSize)/2)).Push(gtx.Ops)
		spinGtx := gtx
		spinGtx.Constraints = layout.Exact(image.Pt(spinSize, spinSize))
		l := material.Loader(t.theme)
		l.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 220}
		l.Layout(spinGtx)
		off.Pop()
		gtx.Execute(op.InvalidateCmd{})
	}

	return layout.Dimensions{Size: sz}
}
