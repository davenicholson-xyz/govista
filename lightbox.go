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
func (s *state) drawLightbox(gtx layout.Context) {
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
			Fit:      widget.Contain,
			Position: layout.Center,
		}.Layout(imgGtx)
	}

	// Info overlay at the bottom.
	s.drawLightboxInfo(gtx, detail)
}

// drawLightboxInfo renders the info band over the bottom of the image.
func (s *state) drawLightboxInfo(gtx layout.Context, detail *wh.Wallpaper) {
	W := gtx.Constraints.Max.X
	H := gtx.Constraints.Max.Y
	pad := gtx.Dp(unit.Dp(24))

	infoH := gtx.Dp(unit.Dp(180))
	yTop := H - infoH
	if yTop < 0 {
		yTop = 0
	}

	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 8, G: 8, B: 8, A: 210},
		clip.Rect{Min: image.Pt(0, yTop), Max: image.Pt(W, H)}.Op(),
	)

	if detail == nil {
		off := op.Offset(image.Pt(pad, yTop+gtx.Dp(unit.Dp(12)))).Push(gtx.Ops)
		lbl := material.Label(s.theme, unit.Sp(13), "Loading…")
		lbl.Color = color.NRGBA{R: 120, G: 120, B: 120, A: 255}
		lbl.Layout(gtx)
		off.Pop()
		return
	}

	// Content starts near the top of the info band.
	y := yTop + gtx.Dp(unit.Dp(12))
	lineGap := gtx.Dp(unit.Dp(8))

	// ── Row 1: resolution + ratio (left)  |  views + favorites (right) ──
	{
		// Resolution (large, white).
		res := detail.Resolution
		off := op.Offset(image.Pt(pad, y)).Push(gtx.Ops)
		resLbl := material.Label(s.theme, unit.Sp(18), res)
		resLbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		resLbl.Layout(gtx)
		off.Pop()

		// Ratio (small, dimmed), positioned after resolution text.
		// Approximate character width to offset; just place it inline.
		if detail.Ratio != "" {
			// measure resolution label width
			macro := op.Record(gtx.Ops)
			tGtx := gtx
			tGtx.Constraints = layout.Constraints{Max: image.Pt(W, gtx.Dp(unit.Dp(30)))}
			resMacroLbl := material.Label(s.theme, unit.Sp(18), res)
			resMacroLbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			resDims := resMacroLbl.Layout(tGtx)
			macro.Stop() // discard

			ratioOff := op.Offset(image.Pt(pad+resDims.Size.X+gtx.Dp(unit.Dp(8)), y+gtx.Dp(unit.Dp(4)))).Push(gtx.Ops)
			ratioLbl := material.Label(s.theme, unit.Sp(12), detail.Ratio)
			ratioLbl.Color = color.NRGBA{R: 180, G: 180, B: 180, A: 150}
			ratioLbl.Layout(gtx)
			ratioOff.Pop()
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
			statsOff := op.Offset(image.Pt(0, y+gtx.Dp(unit.Dp(4)))).Push(gtx.Ops)
			sGtx := gtx
			sGtx.Constraints = layout.Exact(image.Pt(W-pad, gtx.Dp(unit.Dp(20))))
			sLbl := material.Label(s.theme, unit.Sp(12), statsText)
			sLbl.Color = color.NRGBA{R: 180, G: 180, B: 180, A: 200}
			layout.E.Layout(sGtx, sLbl.Layout)
			statsOff.Pop()
		}

		y += gtx.Dp(unit.Dp(26)) + lineGap
	}

	// ── Row 2: chips (category, purity, file type, file size) + color swatches ──
	{
		x := pad
		chipH := gtx.Dp(unit.Dp(20))

		drawChip := func(label string, accent bool) {
			chipPadX := gtx.Dp(unit.Dp(7))
			chipPadY := gtx.Dp(unit.Dp(3))

			macro := op.Record(gtx.Ops)
			tGtx := gtx
			tGtx.Constraints = layout.Constraints{Max: image.Pt(W, chipH)}
			var lbl material.LabelStyle
			if accent {
				lbl = material.Label(s.theme, unit.Sp(11), label)
				lbl.Color = accentColor
			} else {
				lbl = material.Label(s.theme, unit.Sp(11), label)
				lbl.Color = color.NRGBA{R: 200, G: 200, B: 200, A: 180}
			}
			dims := lbl.Layout(tGtx)
			call := macro.Stop()

			cw := dims.Size.X + 2*chipPadX
			var bgColor color.NRGBA
			if accent {
				bgColor = color.NRGBA{R: 124, G: 106, B: 247, A: 40}
			} else {
				bgColor = color.NRGBA{R: 255, G: 255, B: 255, A: 25}
			}
			paint.FillShape(gtx.Ops, bgColor,
				clip.RRect{
					Rect: image.Rect(x, y, x+cw, y+chipH),
					NE: 4, NW: 4, SE: 4, SW: 4,
				}.Op(gtx.Ops),
			)
			off := op.Offset(image.Pt(x+chipPadX, y+chipPadY)).Push(gtx.Ops)
			call.Add(gtx.Ops)
			off.Pop()
			x += cw + gtx.Dp(unit.Dp(5))
		}

		if detail.Category != "" {
			drawChip(detail.Category, true)
		}
		if detail.Purity != "" {
			drawChip(detail.Purity, false)
		}
		if detail.FileType != "" {
			ext := strings.ToUpper(strings.TrimPrefix(detail.FileType, "image/"))
			drawChip(ext, false)
		}
		if detail.FileSize > 0 {
			drawChip(fmtBytes(detail.FileSize), false)
		}

		// Color swatches.
		if len(detail.Colors) > 0 {
			x += gtx.Dp(unit.Dp(4))
			swSz := gtx.Dp(unit.Dp(14))
			for _, hex := range detail.Colors {
				c := parseHexColor(hex)
				paint.FillShape(gtx.Ops, c,
					clip.RRect{
						Rect: image.Rect(x, y+(chipH-swSz)/2, x+swSz, y+(chipH-swSz)/2+swSz),
						NE: 3, NW: 3, SE: 3, SW: 3,
					}.Op(gtx.Ops),
				)
				// subtle border
				paint.FillShape(gtx.Ops,
					color.NRGBA{R: 255, G: 255, B: 255, A: 30},
					clip.Rect{Min: image.Pt(x, y+(chipH-swSz)/2), Max: image.Pt(x+1, y+(chipH-swSz)/2+swSz)}.Op(),
				)
				x += swSz + gtx.Dp(unit.Dp(4))
			}
		}

		y += chipH + lineGap
	}

	// ── Row 3: tags (horizontal scrolling list, navigable with h/l) ──
	if len(detail.Tags) > 0 {
		tagH := gtx.Dp(unit.Dp(22))
		off := op.Offset(image.Pt(pad, y)).Push(gtx.Ops)
		tGtx := gtx
		tGtx.Constraints = layout.Exact(image.Pt(W-2*pad, tagH))
		s.lbTagList.Layout(tGtx, len(detail.Tags), func(gtx layout.Context, i int) layout.Dimensions {
			return s.drawTagChip(gtx, detail.Tags[i].Name, i == s.lbTagIdx)
		})
		off.Pop()
		y += tagH + lineGap
	}

	// ── Row 4: meta (ID · uploader · date) ──
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
			off := op.Offset(image.Pt(pad, y)).Push(gtx.Ops)
			mLbl := material.Label(s.theme, unit.Sp(11), strings.Join(parts, " · "))
			mLbl.Color = color.NRGBA{R: 130, G: 130, B: 130, A: 200}
			mLbl.Layout(gtx)
			off.Pop()
		}
	}
}

// drawTagChip renders a single tag chip for the horizontal list.
func (s *state) drawTagChip(gtx layout.Context, name string, selected bool) layout.Dimensions {
	padX := gtx.Dp(unit.Dp(6))
	padY := gtx.Dp(unit.Dp(3))
	chipH := gtx.Dp(unit.Dp(22))
	gap := gtx.Dp(unit.Dp(5))

	// Record label to measure it.
	macro := op.Record(gtx.Ops)
	tGtx := gtx
	tGtx.Constraints = layout.Constraints{Max: image.Pt(1 << 20, chipH)}
	lbl := material.Label(s.theme, unit.Sp(11), name)
	if selected {
		lbl.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	} else {
		lbl.Color = color.NRGBA{R: 200, G: 200, B: 200, A: 130}
	}
	dims := lbl.Layout(tGtx)
	call := macro.Stop()

	chipW := dims.Size.X + 2*padX

	var bgColor color.NRGBA
	if selected {
		bgColor = color.NRGBA{R: 255, G: 255, B: 255, A: 55}
	} else {
		bgColor = color.NRGBA{R: 255, G: 255, B: 255, A: 20}
	}
	paint.FillShape(gtx.Ops, bgColor,
		clip.RRect{
			Rect: image.Rect(0, 0, chipW, chipH),
			NE: 3, NW: 3, SE: 3, SW: 3,
		}.Op(gtx.Ops),
	)
	if selected {
		// Subtle white outline.
		bw := 1
		bc := color.NRGBA{R: 255, G: 255, B: 255, A: 120}
		paint.FillShape(gtx.Ops, bc, clip.Rect{Max: image.Pt(chipW, bw)}.Op())
		paint.FillShape(gtx.Ops, bc, clip.Rect{Min: image.Pt(0, chipH-bw), Max: image.Pt(chipW, chipH)}.Op())
		paint.FillShape(gtx.Ops, bc, clip.Rect{Max: image.Pt(bw, chipH)}.Op())
		paint.FillShape(gtx.Ops, bc, clip.Rect{Min: image.Pt(chipW-bw, 0), Max: image.Pt(chipW, chipH)}.Op())
	}

	off := op.Offset(image.Pt(padX, padY)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()

	return layout.Dimensions{Size: image.Pt(chipW+gap, chipH)}
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
