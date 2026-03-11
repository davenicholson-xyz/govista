package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math/rand"
	"os"
	"sync"

	wh "github.com/davenicholson-xyz/go-wallhaven/wallhavenapi"
	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func main() {
	go func() {
		w := new(app.Window)
		w.Option(app.Title("GoVista"))
		if err := run(w); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

// keyTag is a zero-size type used as the tag for global keyboard events.
type keyTag struct{}

type state struct {
	mu      sync.Mutex
	thumbs  []*Thumb
	list    layout.List

	// Config + active query state.
	cfg      Config
	sorting  string    // active sorting (may change via keybindings)
	seed     string    // random seed (for pagination consistency)
	srchQ    string    // active search query
	queryObj *wh.Query // current query, rebuilt on sorting/search change

	// Pagination.
	page     int
	lastPage int
	loading  bool

	// Grid navigation — render-thread only, no mutex needed.
	selected int
	cols     int

	// Search modal.
	searchOpen bool
	searchText string

	// Collections modal.
	collectOpen    bool
	collections    []wh.Collection
	collSelected   int
	collLoading    bool
	collLabel      string // name of the active collection

	// Keyboard.
	kt keyTag

	// Text rendering.
	theme *material.Theme
}

func run(w *app.Window) error {
	cfg := loadConfig()
	parseFlags(&cfg)

	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))

	sorting := cfg.Sorting
	seed := ""
	if sorting == "random" {
		seed = newSeed()
	}

	s := &state{
		list:     layout.List{Axis: layout.Vertical},
		cfg:      cfg,
		sorting:  sorting,
		seed:     seed,
		srchQ:    cfg.Query,
		selected: -1,
		theme:    th,
	}
	s.queryObj = buildQuery(cfg, sorting, cfg.Query, seed)

	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			s.layout(gtx, w)
			e.Frame(gtx.Ops)
		}
	}
}

// newSeed generates a 6-character random alphanumeric seed for random sorting.
func newSeed() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

const maxCellDp = unit.Dp(200)

var accentColor = color.NRGBA{R: 124, G: 106, B: 247, A: 255}

func (s *state) layout(gtx layout.Context, w *app.Window) layout.Dimensions {
	// 1. Dark background.
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 18, G: 18, B: 18, A: 255},
		clip.Rect{Max: gtx.Constraints.Max}.Op(),
	)

	// 2. Register global keyboard event handler over the whole window.
	fullArea := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &s.kt)
	fullArea.Pop()

	// Claim focus for the global handler on the first frame and whenever
	// it has been lost (e.g. nothing else should steal it in this app).
	if !gtx.Focused(&s.kt) {
		gtx.Execute(key.FocusCmd{Tag: &s.kt})
	}

	// 3. Process all keyboard events accumulated since the last frame.
	s.handleKeys(gtx, w)

	// 4. Draw the thumbnail grid.
	s.mu.Lock()
	thumbs := s.thumbs
	s.mu.Unlock()

	maxCellPx := gtx.Dp(maxCellDp)
	cols := gtx.Constraints.Max.X / maxCellPx
	if cols < 1 {
		cols = 1
	}
	s.cols = cols // stored for keyboard navigation (same goroutine)

	n := len(thumbs)
	rows := (n + cols - 1) / cols

	dims := s.list.Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
		return s.layoutRow(gtx, w, thumbs, row, cols, n)
	})

	// 5. Trigger next page when within 3 rows of the end.
	s.mu.Lock()
	nearEnd := rows == 0 || s.list.Position.First+s.list.Position.Count+3 >= rows
	canLoad := !s.loading && (s.lastPage == 0 || s.page < s.lastPage)
	s.mu.Unlock()

	if nearEnd && canLoad {
		s.loadNextPage(w)
	}

	// 6. Overlays drawn on top of the grid.
	s.drawStatus(gtx)
	if s.searchOpen {
		s.drawSearch(gtx)
	}
	if s.collectOpen {
		s.drawCollections(gtx)
	}

	return dims
}

func (s *state) layoutRow(gtx layout.Context, w *app.Window, thumbs []*Thumb, row, cols, total int) layout.Dimensions {
	children := make([]layout.FlexChild, cols)
	for c := 0; c < cols; c++ {
		idx := row*cols + c
		if idx < total {
			t := thumbs[idx]
			sel := idx == s.selected
			children[c] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return t.layout(gtx, w, sel)
				})
			})
		} else {
			children[c] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				cw := gtx.Constraints.Max.X
				return layout.Dimensions{Size: image.Pt(cw, cw*9/16)}
			})
		}
	}
	return layout.Flex{}.Layout(gtx, children...)
}

// handleKeys reads all key events for this frame and dispatches them.
func (s *state) handleKeys(gtx layout.Context, w *app.Window) {
	// Enable text-input mode (delivers key.EditEvent for typed characters).
	if s.searchOpen {
		key.InputHintOp{Tag: &s.kt, Hint: key.HintAny}.Add(gtx.Ops)
	}

	searchJustOpened := false
	for {
		e, ok := gtx.Event(
			key.FocusFilter{Target: &s.kt}, // FocusEvent + EditEvent
			// Navigation — no modifier = plain lowercase key.
			key.Filter{Focus: &s.kt, Name: "H"},
			key.Filter{Focus: &s.kt, Name: "J"},
			key.Filter{Focus: &s.kt, Name: "K"},
			key.Filter{Focus: &s.kt, Name: "L"},
			key.Filter{Focus: &s.kt, Name: key.NameLeftArrow},
			key.Filter{Focus: &s.kt, Name: key.NameRightArrow},
			key.Filter{Focus: &s.kt, Name: key.NameUpArrow},
			key.Filter{Focus: &s.kt, Name: key.NameDownArrow},
			// Sorting — Shift variants.
			key.Filter{Focus: &s.kt, Name: "H", Required: key.ModShift},
			key.Filter{Focus: &s.kt, Name: "L", Required: key.ModShift},
			key.Filter{Focus: &s.kt, Name: "T", Required: key.ModShift},
			key.Filter{Focus: &s.kt, Name: "R", Required: key.ModShift},
			// Search open.
			key.Filter{Focus: &s.kt, Name: "S", Required: key.ModShift},
			key.Filter{Focus: &s.kt, Name: "/"},
			// Collections open.
			key.Filter{Focus: &s.kt, Name: "C", Required: key.ModShift},
			// Universal actions.
			key.Filter{Focus: &s.kt, Name: key.NameReturn},
			key.Filter{Focus: &s.kt, Name: key.NameEscape},
			key.Filter{Focus: &s.kt, Name: "Q"},
			// Backspace for search editing.
			key.Filter{Focus: &s.kt, Name: key.NameDeleteBackward},
		)
		if !ok {
			break
		}

		switch ev := e.(type) {
		case key.EditEvent:
			// Typed characters — only append when search is open.
			// Skip the first EditEvent in the frame that opened the search
			// (the triggering key 'S' or '/' would otherwise appear in the box).
			if s.searchOpen && !searchJustOpened {
				s.searchText += ev.Text
			}

		case key.Event:
			if ev.State != key.Press {
				continue
			}

			if s.searchOpen {
				switch ev.Name {
				case key.NameReturn:
					q := s.searchText
					s.searchOpen = false
					s.searchText = ""
					if q != "" {
						s.applySearch(q, w)
					}
				case key.NameEscape:
					s.searchOpen = false
					s.searchText = ""
				case key.NameDeleteBackward:
					runes := []rune(s.searchText)
					if len(runes) > 0 {
						s.searchText = string(runes[:len(runes)-1])
					}
				}
				continue
			}

			if s.collectOpen {
				switch ev.Name {
				case "J", key.NameDownArrow:
					if s.collSelected < len(s.collections)-1 {
						s.collSelected++
					}
				case "K", key.NameUpArrow:
					if s.collSelected > 0 {
						s.collSelected--
					}
				case key.NameReturn:
					if s.collSelected >= 0 && s.collSelected < len(s.collections) {
						coll := s.collections[s.collSelected]
						s.collectOpen = false
						s.applyCollection(coll, w)
					}
				case key.NameEscape:
					s.collectOpen = false
				}
				continue
			}

			// Normal mode.
			shift := ev.Modifiers.Contain(key.ModShift)
			switch {
			case ev.Name == "H" && shift:
				s.applySorting("hot", w)
			case ev.Name == "T" && shift:
				s.applySorting("toplist", w)
			case ev.Name == "L" && shift:
				s.applySorting("date_added", w)
			case ev.Name == "R" && shift:
				s.applySorting("random", w)
			case (ev.Name == "S" && shift) || ev.Name == "/":
				s.searchOpen = true
				s.searchText = ""
				searchJustOpened = true
			case ev.Name == "C" && shift:
				s.openCollections(w)
			case ev.Name == "Q" || ev.Name == key.NameEscape:
				w.Perform(system.ActionClose)
			case ev.Name == key.NameReturn:
				s.activateSelected(w)
			case ev.Name == "H" || ev.Name == key.NameLeftArrow:
				s.navigate(-1, 0)
			case ev.Name == "L" || ev.Name == key.NameRightArrow:
				s.navigate(1, 0)
			case ev.Name == "K" || ev.Name == key.NameUpArrow:
				s.navigate(0, -1)
			case ev.Name == "J" || ev.Name == key.NameDownArrow:
				s.navigate(0, 1)
			}
		}
	}
}

// navigate moves the keyboard selection by (dx, dy) cells and scrolls the
// list to keep the selected row visible.
func (s *state) navigate(dx, dy int) {
	s.mu.Lock()
	n := len(s.thumbs)
	s.mu.Unlock()

	if n == 0 {
		return
	}
	cols := s.cols
	if cols < 1 {
		cols = 1
	}
	if s.selected < 0 {
		s.selected = 0
		return
	}

	row := s.selected / cols
	col := s.selected % cols
	newCol := col + dx
	newRow := row + dy

	if newCol < 0 || newCol >= cols {
		return
	}
	newIdx := newRow*cols + newCol
	if newIdx < 0 || newIdx >= n {
		return
	}
	s.selected = newIdx

	selRow := newIdx / cols
	pos := s.list.Position
	if selRow < pos.First {
		// Row is above the viewport — snap up to it.
		s.list.ScrollTo(selRow)
	} else if pos.Count > 0 && pos.BeforeEnd && selRow >= pos.First+pos.Count-1 {
		// Selection has reached the last visible row (which may be partially
		// cut off). Scroll down by one to keep it fully visible.
		s.list.Position.First = pos.First + 1
		s.list.Position.Offset = 0
		s.list.Position.BeforeEnd = true
	}
}

// activateSelected downloads and sets the wallpaper for the selected cell.
func (s *state) activateSelected(w *app.Window) {
	s.mu.Lock()
	thumbs := s.thumbs
	s.mu.Unlock()

	if s.selected < 0 || s.selected >= len(thumbs) {
		return
	}
	t := thumbs[s.selected]
	id, url, cfg := t.ID, t.FullURL, s.cfg
	go func() {
		if err := downloadAndSet(id, url, cfg, w); err != nil {
			log.Println("govista: set wallpaper:", err)
		}
	}()
}

// applySorting resets the gallery with a new sort mode.
func (s *state) applySorting(sorting string, w *app.Window) {
	seed := ""
	if sorting == "random" {
		seed = newSeed()
	}
	s.mu.Lock()
	s.sorting = sorting
	s.seed = seed
	s.srchQ = s.cfg.Query
	s.collLabel = ""
	s.queryObj = buildQuery(s.cfg, sorting, s.cfg.Query, seed)
	s.thumbs = nil
	s.page = 0
	s.lastPage = 0
	s.loading = false
	s.mu.Unlock()
	s.selected = -1
	w.Invalidate()
}

// applySearch resets the gallery with a new search query.
func (s *state) applySearch(query string, w *app.Window) {
	s.mu.Lock()
	s.sorting = "relevance"
	s.seed = ""
	s.srchQ = query
	s.collLabel = ""
	s.queryObj = buildQuery(s.cfg, "relevance", query, "")
	s.thumbs = nil
	s.page = 0
	s.lastPage = 0
	s.loading = false
	s.mu.Unlock()
	s.selected = -1
	w.Invalidate()
}

// loadNextPage fetches the next page of wallpapers in a background goroutine.
func (s *state) loadNextPage(w *app.Window) {
	s.mu.Lock()
	if s.loading {
		s.mu.Unlock()
		return
	}
	s.loading = true
	nextPage := s.page + 1
	q := s.queryObj
	s.mu.Unlock()

	go func() {
		thumbs, lastPage, err := fetchPage(q, nextPage)
		if err != nil {
			log.Println("govista: fetch error:", err)
			s.mu.Lock()
			s.loading = false
			s.mu.Unlock()
			return
		}
		s.mu.Lock()
		cfg := s.cfg
		for _, t := range thumbs {
			t.cfg = cfg
		}
		s.thumbs = append(s.thumbs, thumbs...)
		s.page = nextPage
		s.lastPage = lastPage
		s.loading = false
		s.mu.Unlock()
		for _, t := range thumbs {
			go t.load(w)
		}
		w.Invalidate()
	}()
}

// sortingLabel maps internal sorting keys to human-readable names.
func sortingLabel(sorting string) string {
	switch sorting {
	case "hot":
		return "hot"
	case "toplist":
		return "toplist"
	case "random":
		return "random"
	case "relevance":
		return "search"
	case "collection":
		return "collection"
	default:
		return "latest"
	}
}

// drawStatus renders a small status badge in the bottom-right corner.
func (s *state) drawStatus(gtx layout.Context) {
	s.mu.Lock()
	sorting := s.sorting
	srchQ := s.srchQ
	collLabel := s.collLabel
	page := s.page
	lastPage := s.lastPage
	s.mu.Unlock()

	label := sortingLabel(sorting)
	switch sorting {
	case "relevance":
		if srchQ != "" {
			label += ": " + srchQ
		}
	case "collection":
		if collLabel != "" {
			label += ": " + collLabel
		}
	}
	if page > 0 {
		label += fmt.Sprintf(" · %d/%d", page, lastPage)
	}

	gtx2 := gtx
	gtx2.Constraints = layout.Exact(gtx.Constraints.Max)
	layout.SE.Layout(gtx2, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// Record dims first, then draw background, then replay text on top.
			macro := op.Record(gtx.Ops)
			lbl := material.Label(s.theme, unit.Sp(11), label)
			lbl.Color = color.NRGBA{R: 122, G: 122, B: 122, A: 255}
			dims := lbl.Layout(gtx)
			call := macro.Stop()

			pad := gtx.Dp(unit.Dp(6))
			bg := image.Rect(-pad, -pad/2, dims.Size.X+pad, dims.Size.Y+pad/2)
			paint.FillShape(gtx.Ops,
				color.NRGBA{R: 26, G: 26, B: 26, A: 210},
				clip.RRect{Rect: bg, NE: 4, NW: 4, SE: 4, SW: 4}.Op(gtx.Ops),
			)
			call.Add(gtx.Ops)
			return dims
		})
	})
}

// drawSearch renders the search modal overlay.
func (s *state) drawSearch(gtx layout.Context) {
	// Semi-transparent backdrop.
	paint.FillShape(gtx.Ops,
		color.NRGBA{A: 140},
		clip.Rect{Max: gtx.Constraints.Max}.Op(),
	)

	boxW := min(gtx.Dp(unit.Dp(500)), gtx.Constraints.Max.X-gtx.Dp(unit.Dp(32)))
	boxH := gtx.Dp(unit.Dp(50))
	boxX := (gtx.Constraints.Max.X - boxW) / 2
	boxY := gtx.Constraints.Max.Y*2/5 - boxH/2

	// Box background.
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 26, G: 26, B: 26, A: 255},
		clip.RRect{
			Rect: image.Rect(boxX, boxY, boxX+boxW, boxY+boxH),
			NE:   10, NW: 10, SE: 10, SW: 10,
		}.Op(gtx.Ops),
	)

	// Accent border (1 px).
	bw := 1
	bc := color.NRGBA{R: 124, G: 106, B: 247, A: 180}
	ox, oy := boxX-bw, boxY-bw
	ow, oh := boxW+bw*2, boxH+bw*2
	paint.FillShape(gtx.Ops, bc, clip.Rect{Min: image.Pt(ox, oy), Max: image.Pt(ox+ow, boxY)}.Op())
	paint.FillShape(gtx.Ops, bc, clip.Rect{Min: image.Pt(ox, boxY+boxH), Max: image.Pt(ox+ow, oy+oh)}.Op())
	paint.FillShape(gtx.Ops, bc, clip.Rect{Min: image.Pt(ox, boxY), Max: image.Pt(boxX, boxY+boxH)}.Op())
	paint.FillShape(gtx.Ops, bc, clip.Rect{Min: image.Pt(boxX+boxW, boxY), Max: image.Pt(ox+ow, boxY+boxH)}.Op())

	// Search text + cursor, or placeholder.
	textPad := gtx.Dp(unit.Dp(16))
	off := op.Offset(image.Pt(boxX+textPad, boxY)).Push(gtx.Ops)
	textGtx := gtx
	textGtx.Constraints = layout.Exact(image.Pt(boxW-2*textPad, boxH))

	var displayText string
	var textColor color.NRGBA
	if s.searchText == "" {
		displayText = "Search wallpapers… │"
		textColor = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
	} else {
		displayText = s.searchText + "│"
		textColor = color.NRGBA{R: 232, G: 232, B: 232, A: 255}
	}
	lbl := material.Label(s.theme, unit.Sp(15), displayText)
	lbl.Color = textColor
	layout.Center.Layout(textGtx, lbl.Layout)
	off.Pop()

	// Hint text below the box.
	hintOff := op.Offset(image.Pt(boxX, boxY+boxH+gtx.Dp(unit.Dp(10)))).Push(gtx.Ops)
	hintGtx := gtx
	hintGtx.Constraints = layout.Exact(image.Pt(boxW, gtx.Dp(unit.Dp(20))))
	hintLbl := material.Label(s.theme, unit.Sp(11), "Enter to search  ·  Esc to cancel")
	hintLbl.Color = color.NRGBA{R: 80, G: 80, B: 80, A: 200}
	layout.Center.Layout(hintGtx, hintLbl.Layout)
	hintOff.Pop()
}

// openCollections fetches the user's collections asynchronously and opens the modal.
func (s *state) openCollections(w *app.Window) {
	if s.collLoading {
		return
	}
	s.collLoading = true
	s.collections = nil
	s.collSelected = 0
	cfg := s.cfg
	go func() {
		colls, err := fetchCollections(cfg)
		s.mu.Lock()
		s.collLoading = false
		if err != nil {
			log.Println("govista: collections:", err)
			s.mu.Unlock()
			return
		}
		s.collections = colls
		if len(colls) != 1 {
			s.collectOpen = true
		}
		s.mu.Unlock()
		if len(colls) == 1 {
			s.applyCollection(colls[0], w)
			return
		}
		w.Invalidate()
	}()
}

// applyCollection resets the gallery to show wallpapers from a collection.
func (s *state) applyCollection(coll wh.Collection, w *app.Window) {
	username := s.cfg.Username
	s.mu.Lock()
	s.sorting = "collection"
	s.seed = ""
	s.srchQ = ""
	s.collLabel = coll.Label
	s.queryObj = buildCollectionQuery(s.cfg, username, coll.ID)
	s.thumbs = nil
	s.page = 0
	s.lastPage = 0
	s.loading = false
	s.mu.Unlock()
	s.selected = -1
	w.Invalidate()
}

// drawCollections renders the collections picker modal.
func (s *state) drawCollections(gtx layout.Context) {
	// Semi-transparent backdrop.
	paint.FillShape(gtx.Ops,
		color.NRGBA{A: 140},
		clip.Rect{Max: gtx.Constraints.Max}.Op(),
	)

	s.mu.Lock()
	colls := s.collections
	collSel := s.collSelected
	loading := s.collLoading
	s.mu.Unlock()

	rowH := gtx.Dp(unit.Dp(38))
	boxW := min(gtx.Dp(unit.Dp(500)), gtx.Constraints.Max.X-gtx.Dp(unit.Dp(32)))
	boxH := rowH * max(len(colls), 1)
	boxX := (gtx.Constraints.Max.X - boxW) / 2
	boxY := gtx.Constraints.Max.Y*2/5 - boxH/2

	// Box background.
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 26, G: 26, B: 26, A: 255},
		clip.RRect{
			Rect: image.Rect(boxX, boxY, boxX+boxW, boxY+boxH),
			NE:   10, NW: 10, SE: 10, SW: 10,
		}.Op(gtx.Ops),
	)

	// Accent border (1 px).
	bw := 1
	bc := color.NRGBA{R: 124, G: 106, B: 247, A: 180}
	ox, oy := boxX-bw, boxY-bw
	ow, oh := boxW+bw*2, boxH+bw*2
	paint.FillShape(gtx.Ops, bc, clip.Rect{Min: image.Pt(ox, oy), Max: image.Pt(ox+ow, boxY)}.Op())
	paint.FillShape(gtx.Ops, bc, clip.Rect{Min: image.Pt(ox, boxY+boxH), Max: image.Pt(ox+ow, oy+oh)}.Op())
	paint.FillShape(gtx.Ops, bc, clip.Rect{Min: image.Pt(ox, boxY), Max: image.Pt(boxX, boxY+boxH)}.Op())
	paint.FillShape(gtx.Ops, bc, clip.Rect{Min: image.Pt(boxX+boxW, boxY), Max: image.Pt(ox+ow, boxY+boxH)}.Op())

	textPad := gtx.Dp(unit.Dp(16))

	if loading || len(colls) == 0 {
		msg := "Loading collections…"
		if !loading {
			msg = "No collections found"
		}
		off := op.Offset(image.Pt(boxX+textPad, boxY)).Push(gtx.Ops)
		tGtx := gtx
		tGtx.Constraints = layout.Exact(image.Pt(boxW-2*textPad, rowH))
		lbl := material.Label(s.theme, unit.Sp(14), msg)
		lbl.Color = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
		layout.W.Layout(tGtx, lbl.Layout)
		off.Pop()
	} else {
		for i, coll := range colls {
			y := boxY + i*rowH
			// Highlight selected row.
			if i == collSel {
				paint.FillShape(gtx.Ops,
					color.NRGBA{R: 124, G: 106, B: 247, A: 40},
					clip.Rect{Min: image.Pt(boxX, y), Max: image.Pt(boxX+boxW, y+rowH)}.Op(),
				)
				// Left accent bar.
				paint.FillShape(gtx.Ops,
					accentColor,
					clip.Rect{Min: image.Pt(boxX, y), Max: image.Pt(boxX+gtx.Dp(unit.Dp(3)), y+rowH)}.Op(),
				)
			}
			off := op.Offset(image.Pt(boxX+textPad, y)).Push(gtx.Ops)
			tGtx := gtx
			tGtx.Constraints = layout.Exact(image.Pt(boxW-2*textPad, rowH))

			label := fmt.Sprintf("%s  (%d)", coll.Label, coll.Count)
			textColor := color.NRGBA{R: 180, G: 180, B: 180, A: 255}
			if i == collSel {
				textColor = color.NRGBA{R: 232, G: 232, B: 232, A: 255}
			}
			lbl := material.Label(s.theme, unit.Sp(14), label)
			lbl.Color = textColor
			layout.W.Layout(tGtx, lbl.Layout)
			off.Pop()
		}
	}

	// Hint text below the box.
	hintOff := op.Offset(image.Pt(boxX, boxY+boxH+gtx.Dp(unit.Dp(10)))).Push(gtx.Ops)
	hintGtx := gtx
	hintGtx.Constraints = layout.Exact(image.Pt(boxW, gtx.Dp(unit.Dp(20))))
	hintLbl := material.Label(s.theme, unit.Sp(11), "j/k to navigate  ·  Enter to open  ·  Esc to cancel")
	hintLbl.Color = color.NRGBA{R: 80, G: 80, B: 80, A: 200}
	layout.Center.Layout(hintGtx, hintLbl.Layout)
	hintOff.Pop()
}
