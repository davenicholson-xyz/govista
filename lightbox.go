package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"net/http"
	"strconv"
	"strings"

	wh "github.com/davenicholson-xyz/go-wallhaven/wallhavenapi"
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

// openLightbox opens the lightbox for the given thumbnail.
// It immediately displays the already-loaded thumb image as a placeholder,
// then fetches the full-res image and wallpaper detail in the background.
func (s *state) openLightbox(t *Thumb, w *app.Window) {
	s.mu.Lock()
	s.lbVersion++
	ver := s.lbVersion
	id := t.ID
	fullURL := t.FullURL
	cfg := s.cfg

	// Use the thumb's already-loaded image as an immediate placeholder.
	t.mu.Lock()
	var initImg image.Image
	if t.loaded {
		initImg = t.img
	}
	t.mu.Unlock()

	s.lbImg = initImg
	s.lbDetail = nil
	s.lbOpen = true
	s.lbThumb = t
	s.mu.Unlock()

	s.lbTagIdx = -1

	// Fetch full-res image in background.
	go func() {
		resp, err := http.Get(fullURL)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		img, _, err := image.Decode(resp.Body)
		if err != nil {
			return
		}
		s.mu.Lock()
		if s.lbVersion == ver {
			s.lbImg = img
		}
		s.mu.Unlock()
		w.Invalidate()
	}()

	// Fetch wallpaper metadata.
	go func() {
		var client *wh.WallhavenAPI
		if cfg.APIKey != "" {
			client = wh.NewWithAPIKey(cfg.APIKey)
		} else {
			client = wh.New()
		}
		detail, err := client.Wallpaper(id)
		if err != nil {
			log.Println("govista: lightbox detail:", err)
			return
		}
		s.mu.Lock()
		if s.lbVersion == ver {
			s.lbDetail = &detail
		}
		s.mu.Unlock()
		w.Invalidate()
	}()
}

// drawLightbox renders the full-screen lightbox overlay:
// the image fills the window (contain), with an info overlay at the bottom.
func (s *state) drawLightbox(gtx layout.Context, w *app.Window) {
	// Block all pointer events from reaching the grid below.
	pArea := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &s.lbBlockTag)
	pArea.Pop()

	for {
		ev, ok := gtx.Source.Event(pointer.Filter{
			Target: &s.lbCloseTag,
			Kinds:  pointer.Press,
		})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && e.Buttons.Contain(pointer.ButtonSecondary) {
			s.lbOpen = false
			s.lbTagIdx = -1
		}
	}

	// Dark backdrop.
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 10, G: 10, B: 10, A: 230},
		clip.Rect{Max: gtx.Constraints.Max}.Op(),
	)

	s.mu.Lock()
	img := s.lbImg
	detail := s.lbDetail
	s.mu.Unlock()

	// Lazily create/update ImageOp when image changes.
	if img != nil && img != s.lbImgPtr {
		s.lbImgOp = paint.NewImageOp(img)
		s.lbImgPtr = img
	}

	// Draw image filling the full window with contain fit.
	if s.lbImgPtr != nil {
		imgGtx := gtx
		imgGtx.Constraints = layout.Exact(gtx.Constraints.Max)
		widget.Image{
			Src:      s.lbImgOp,
			Fit:      widget.Cover,
			Position: layout.Center,
		}.Layout(imgGtx)
	}

	// Register a click over the image area (above the info panel) to set
	// the wallpaper. Must be registered BEFORE the info panel's tag clicks
	// so that tag clicks (registered later, visually on top) take priority
	// for events in their area.
	H := gtx.Constraints.Max.Y
	W := gtx.Constraints.Max.X
	infoH := s.lbComputeInfoHeight(gtx, detail)
	yTop := H - infoH
	if yTop < 0 {
		yTop = 0
	}
	if s.lbImgClick.Clicked(gtx) && s.lbThumb != nil {
		t := s.lbThumb
		s.lbOpen = false
		s.lbTagIdx = -1
		t.startDownload(w)
	}
	imgClickGtx := gtx
	imgClickGtx.Constraints = layout.Exact(image.Pt(W, yTop))
	imgPass := pointer.PassOp{}.Push(gtx.Ops)
	s.lbImgClick.Layout(imgClickGtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(W, yTop)}
	})
	imgPass.Pop()

	// Info overlay at the bottom.
	s.drawLightboxInfo(gtx, detail, w)

	// Register lbCloseTag LAST so it sits on top of all other handlers.
	// PassOp on lbImgClick/tag/footer ensures left-clicks still reach them.
	closeArea := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &s.lbCloseTag)
	closeArea.Pop()
}

// lbComputeInfoHeight returns the pixel height of the info panel so that
// the image click area can be sized to avoid overlapping the tag chips.
func (s *state) lbComputeInfoHeight(gtx layout.Context, detail *wh.Wallpaper) int {
	chipH := gtx.Dp(unit.Dp(22))
	chipPadX := gtx.Dp(unit.Dp(7))
	tagGap := gtx.Dp(unit.Dp(5))
	tagRowGap := gtx.Dp(unit.Dp(5))
	maxTagW := gtx.Constraints.Max.X - 2*gtx.Dp(unit.Dp(24))
	lineGap := gtx.Dp(unit.Dp(8))

	totalTagH := 0
	if detail != nil && len(detail.Tags) > 0 {
		cx, cy := 0, 0
		for _, tag := range detail.Tags {
			m := op.Record(gtx.Ops)
			tGtx := gtx
			tGtx.Constraints = layout.Constraints{Max: image.Pt(1 << 20, chipH)}
			lbl := material.Label(s.theme, unit.Sp(11), tag.Name)
			lbl.Color = color.NRGBA{}
			dims := lbl.Layout(tGtx)
			m.Stop() // measurement only — discard ops
			cw := dims.Size.X + 2*chipPadX
			if cx > 0 && cx+cw > maxTagW {
				cx = 0
				cy += chipH + tagRowGap
			}
			cx += cw + tagGap
		}
		totalTagH = cy + chipH
	}

	fixedH := gtx.Dp(unit.Dp(12)) +
		gtx.Dp(unit.Dp(26)) + lineGap +
		gtx.Dp(unit.Dp(14)) + gtx.Dp(unit.Dp(12))
	tagSectionH := 0
	if totalTagH > 0 {
		tagSectionH = totalTagH + lineGap
	}
	return fixedH + tagSectionH
}

// drawLightboxInfo renders the info band over the bottom of the image.
func (s *state) drawLightboxInfo(gtx layout.Context, detail *wh.Wallpaper, w *app.Window) {
	W := gtx.Constraints.Max.X
	H := gtx.Constraints.Max.Y
	pad := gtx.Dp(unit.Dp(24))
	lineGap := gtx.Dp(unit.Dp(8))

	// Pre-calculate tag wrap layout so we know total height needed.
	type chipPos struct {
		name string
		x, y int
		w    int
	}
	chipH := gtx.Dp(unit.Dp(22))
	chipPadX := gtx.Dp(unit.Dp(7))
	tagGap := gtx.Dp(unit.Dp(5))
	tagRowGap := gtx.Dp(unit.Dp(5))
	maxTagW := W - 2*pad

	var tagChips []chipPos
	var totalTagH int
	if detail != nil && len(detail.Tags) > 0 {
		cx, cy := 0, 0
		for _, tag := range detail.Tags {
			macro := op.Record(gtx.Ops)
			tGtx := gtx
			tGtx.Constraints = layout.Constraints{Max: image.Pt(1 << 20, chipH)}
			lbl := material.Label(s.theme, unit.Sp(11), tag.Name)
			lbl.Color = color.NRGBA{}
			dims := lbl.Layout(tGtx)
			macro.Stop()
			cw := dims.Size.X + 2*chipPadX
			if cx > 0 && cx+cw > maxTagW {
				cx = 0
				cy += chipH + tagRowGap
			}
			tagChips = append(tagChips, chipPos{tag.Name, cx, cy, cw})
			cx += cw + tagGap
		}
		totalTagH = cy + chipH
	}

	// Compute dynamic info band height.
	fixedH := gtx.Dp(unit.Dp(12)) + // top padding
		gtx.Dp(unit.Dp(26)) + lineGap + // row1 (resolution + meta inline)
		gtx.Dp(unit.Dp(14)) + gtx.Dp(unit.Dp(12)) // row4 + bottom padding
	tagSectionH := 0
	if totalTagH > 0 {
		tagSectionH = totalTagH + lineGap
	}
	infoH := fixedH + tagSectionH

	yTop := H - infoH
	if yTop < 0 {
		yTop = 0
	}

	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 8, G: 8, B: 8, A: 220},
		clip.Rect{Min: image.Pt(0, yTop), Max: image.Pt(W, H)}.Op(),
	)
	// Subtle top separator line.
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 255, G: 255, B: 255, A: 18},
		clip.Rect{Min: image.Pt(0, yTop), Max: image.Pt(W, yTop+1)}.Op(),
	)

	if detail == nil {
		off := op.Offset(image.Pt(pad, yTop+gtx.Dp(unit.Dp(12)))).Push(gtx.Ops)
		lbl := material.Label(s.theme, unit.Sp(13), "Loading…")
		lbl.Color = color.NRGBA{R: 120, G: 120, B: 120, A: 255}
		lbl.Layout(gtx)
		off.Pop()
		return
	}

	y := yTop + gtx.Dp(unit.Dp(12))

	// ── Row 1: resolution (left) + meta inline + color swatches  |  views + favorites (right) ──
	{
		rowH := gtx.Dp(unit.Dp(26)) // height driven by resolution text
		subY := y + gtx.Dp(unit.Dp(5)) // vertical center for smaller text

		// Resolution.
		res := detail.Resolution
		macro := op.Record(gtx.Ops)
		tGtx := gtx
		tGtx.Constraints = layout.Constraints{Max: image.Pt(W, rowH)}
		resLbl := material.Label(s.theme, unit.Sp(18), res)
		resLbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		resDims := resLbl.Layout(tGtx)
		resCall := macro.Stop()
		resOff := op.Offset(image.Pt(pad, y)).Push(gtx.Ops)
		resCall.Add(gtx.Ops)
		resOff.Pop()

		// Inline meta: ratio · category · purity · filetype · filesize, plain text.
		x := pad + resDims.Size.X + gtx.Dp(unit.Dp(10))
		drawMeta := func(text string, accent bool) {
			macro := op.Record(gtx.Ops)
			mGtx := gtx
			mGtx.Constraints = layout.Constraints{Max: image.Pt(W, rowH)}
			lbl := material.Label(s.theme, unit.Sp(12), text)
			if accent {
				lbl.Color = color.NRGBA{R: 160, G: 145, B: 255, A: 230}
			} else {
				lbl.Color = color.NRGBA{R: 210, G: 210, B: 210, A: 200}
			}
			dims := lbl.Layout(mGtx)
			call := macro.Stop()
			off := op.Offset(image.Pt(x, subY)).Push(gtx.Ops)
			call.Add(gtx.Ops)
			off.Pop()
			x += dims.Size.X + gtx.Dp(unit.Dp(10))
		}
		drawDot := func() {
			macro := op.Record(gtx.Ops)
			dGtx := gtx
			dGtx.Constraints = layout.Constraints{Max: image.Pt(W, rowH)}
			lbl := material.Label(s.theme, unit.Sp(12), "·")
			lbl.Color = color.NRGBA{R: 100, G: 100, B: 100, A: 180}
			dims := lbl.Layout(dGtx)
			call := macro.Stop()
			off := op.Offset(image.Pt(x, subY)).Push(gtx.Ops)
			call.Add(gtx.Ops)
			off.Pop()
			x += dims.Size.X + gtx.Dp(unit.Dp(10))
		}

		if detail.Ratio != "" {
			drawMeta(detail.Ratio, false)
		}
		if detail.Category != "" {
			drawDot()
			drawMeta(detail.Category, true)
		}
		if detail.Purity != "" {
			drawDot()
			drawMeta(detail.Purity, false)
		}
		if detail.FileType != "" {
			drawDot()
			drawMeta(strings.ToUpper(strings.TrimPrefix(detail.FileType, "image/")), false)
		}
		if detail.FileSize > 0 {
			drawDot()
			drawMeta(fmtBytes(detail.FileSize), false)
		}

		// Color swatches inline.
		if len(detail.Colors) > 0 {
			x += gtx.Dp(unit.Dp(4))
			swSz := gtx.Dp(unit.Dp(11))
			swY := subY + gtx.Dp(unit.Dp(1))
			for _, hex := range detail.Colors {
				c := parseHexColor(hex)
				paint.FillShape(gtx.Ops, c,
					clip.RRect{
						Rect: image.Rect(x, swY, x+swSz, swY+swSz),
						NE: 2, NW: 2, SE: 2, SW: 2,
					}.Op(gtx.Ops),
				)
				x += swSz + gtx.Dp(unit.Dp(3))
			}
		}

		// Views + favorites on the right.
		var statParts []string
		if detail.Views > 0 {
			statParts = append(statParts, fmt.Sprintf("%s views", fmtCount(detail.Views)))
		}
		if detail.Favorites > 0 {
			statParts = append(statParts, fmt.Sprintf("%s ♥", fmtCount(detail.Favorites)))
		}
		if len(statParts) > 0 {
			statsText := strings.Join(statParts, "   ")
			statsOff := op.Offset(image.Pt(0, subY)).Push(gtx.Ops)
			sGtx := gtx
			sGtx.Constraints = layout.Exact(image.Pt(W-pad, gtx.Dp(unit.Dp(20))))
			sLbl := material.Label(s.theme, unit.Sp(12), statsText)
			sLbl.Color = color.NRGBA{R: 180, G: 180, B: 180, A: 200}
			layout.E.Layout(sGtx, sLbl.Layout)
			statsOff.Pop()
		}

		y += rowH + lineGap
	}

	// ── Row 3: tags (wrapped grid, clickable) ──
	if len(tagChips) > 0 {
		// Sync per-tag clickable slice when detail changes.
		if len(s.lbTagClicks) != len(tagChips) {
			s.lbTagClicks = make([]widget.Clickable, len(tagChips))
		}
		// Store chip positions for keyboard navigation.
		if len(s.lbTagChips) != len(tagChips) {
			s.lbTagChips = make([]lbChipPos, len(tagChips))
		}
		for i, c := range tagChips {
			s.lbTagChips[i] = lbChipPos{x: c.x, y: c.y, w: c.w}
		}

		// Check tag clicks before layout (Gio pattern: Clicked before Layout).
		for i, chip := range tagChips {
			if s.lbTagClicks[i].Clicked(gtx) {
				name := chip.name
				s.lbOpen = false
				s.lbTagIdx = -1
				s.applySearch("#"+name, w)
			}
		}

		off := op.Offset(image.Pt(pad, y)).Push(gtx.Ops)
		for i, chip := range tagChips {
			chip := chip // per-iteration capture
			selected := i == s.lbTagIdx
			hovered := s.lbTagClicks[i].Hovered()

			chipOff := op.Offset(image.Pt(chip.x, chip.y)).Push(gtx.Ops)
			chipGtx := gtx
			chipGtx.Constraints = layout.Exact(image.Pt(chip.w, chipH))
			tagPass := pointer.PassOp{}.Push(gtx.Ops)
			s.lbTagClicks[i].Layout(chipGtx, func(gtx layout.Context) layout.Dimensions {
				var textColor color.NRGBA
				if selected || hovered {
					textColor = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
				} else {
					textColor = color.NRGBA{R: 180, G: 180, B: 180, A: 180}
				}
				tOff := op.Offset(image.Pt(chipPadX, 0)).Push(gtx.Ops)
				tGtx := gtx
				tGtx.Constraints = layout.Constraints{Max: image.Pt(chip.w, chipH)}
				lbl := material.Label(s.theme, unit.Sp(11), chip.name)
				lbl.Color = textColor
				dims := lbl.Layout(tGtx)
				tOff.Pop()
				if selected {
					ulY := dims.Size.Y + gtx.Dp(unit.Dp(1))
					paint.FillShape(gtx.Ops,
						color.NRGBA{R: 160, G: 145, B: 255, A: 220},
						clip.Rect{
							Min: image.Pt(chipPadX, ulY),
							Max: image.Pt(chipPadX+dims.Size.X, ulY+gtx.Dp(unit.Dp(1))),
						}.Op(),
					)
				}
				return layout.Dimensions{Size: image.Pt(chip.w, chipH)}
			})
			tagPass.Pop()
			chipOff.Pop()
		}
		off.Pop()
		y += totalTagH + lineGap
	}

	// ── Row 4: meta (ID · uploader · date) — click to open in browser ──
	{
		var parts []string
		if detail.ID != "" {
			parts = append(parts, detail.ID)
		}
		if detail.Uploader.Username != "" {
			parts = append(parts, detail.Uploader.Username)
		}
		if len(detail.CreatedAt) >= 10 {
			parts = append(parts, detail.CreatedAt[:10])
		}
		if len(parts) > 0 {
			if s.lbFooterClick.Clicked(gtx) && s.lbThumb != nil {
				go openInBrowser("https://wallhaven.cc/w/" + s.lbThumb.ID)
			}
			off := op.Offset(image.Pt(pad, y)).Push(gtx.Ops)
			footerGtx := gtx
			footerGtx.Constraints = layout.Constraints{Max: image.Pt(W-2*pad, gtx.Dp(unit.Dp(14)))}
			footerPass := pointer.PassOp{}.Push(gtx.Ops)
			s.lbFooterClick.Layout(footerGtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(s.theme, unit.Sp(11), strings.Join(parts, " · "))
				if s.lbFooterClick.Hovered() {
					lbl.Color = color.NRGBA{R: 180, G: 180, B: 255, A: 230}
				} else {
					lbl.Color = color.NRGBA{R: 130, G: 130, B: 130, A: 200}
				}
				return lbl.Layout(gtx)
			})
			footerPass.Pop()
			off.Pop()
		}
	}
}


// fmtCount formats large numbers with a k suffix (e.g. 12300 → "12.3k").
func fmtCount(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// fmtBytes formats a byte count into a human-readable string.
func fmtBytes(b int) string {
	if b >= 1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	}
	if b >= 1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%d B", b)
}

// parseHexColor converts a "#rrggbb" hex string to color.NRGBA.
func parseHexColor(hex string) color.NRGBA {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return color.NRGBA{A: 255}
	}
	r, _ := strconv.ParseUint(hex[0:2], 16, 8)
	g, _ := strconv.ParseUint(hex[2:4], 16, 8)
	b, _ := strconv.ParseUint(hex[4:6], 16, 8)
	return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
}
