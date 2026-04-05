package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
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
	"gioui.org/widget"
	"gioui.org/widget/material"
)

var version = "version"

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

// lbChipPos records the layout position of a tag chip in the lightbox.
type lbChipPos struct {
	x, y, w int
}

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

	// Help overlay.
	helpOpen bool

	// History view.
	historyOpen     bool
	historyThumbs   []*Thumb
	historyList     layout.List
	historySelected int

	// Lightbox.
	lbOpen      bool
	lbThumb     *Thumb
	lbTagIdx    int // -1 = none selected
	lbVersion   int
	lbTagChips  []lbChipPos // chip positions from last render, render-thread only
	lbBlockTag   struct{}          // pointer event blocker tag
	lbCloseTag   struct{}          // right-click-to-close tag
	lbImgClick   widget.Clickable  // click image area → set wallpaper
	lbTagClicks  []widget.Clickable // one per tag chip
	lbFooterClick widget.Clickable // click footer → open in browser
	// Protected by mu:
	lbDetail *wh.Wallpaper
	lbImg    image.Image
	// Render-thread only:
	lbImgOp  paint.ImageOp
	lbImgPtr image.Image

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
		list:        layout.List{Axis: layout.Vertical},
		historyList: layout.List{Axis: layout.Vertical},
		cfg:       cfg,
		sorting:   sorting,
		seed:      seed,
		srchQ:     cfg.Query,
		selected:  -1,
		theme:     th,
		lbTagIdx: -1,
	}
	s.queryObj = buildQuery(cfg, sorting, cfg.Query, seed)

	pruneCtx, cancelPrune := context.WithCancel(context.Background())
	defer cancelPrune()
	go pruneCache(pruneCtx, int64(cfg.CacheMaxMB)*1024*1024)

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

// openInBrowser opens the given URL in the system default browser.
func openInBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Println("govista: open browser:", err)
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
	var activeThumbs []*Thumb
	var activeList *layout.List
	if s.historyOpen {
		activeThumbs = s.historyThumbs
		activeList = &s.historyList
	} else {
		s.mu.Lock()
		activeThumbs = s.thumbs
		s.mu.Unlock()
		activeList = &s.list
	}

	maxCellPx := gtx.Dp(unit.Dp(s.cfg.ThumbSize))
	cols := gtx.Constraints.Max.X / maxCellPx
	if cols < 1 {
		cols = 1
	}
	s.cols = cols // stored for keyboard navigation (same goroutine)

	n := len(activeThumbs)
	rows := (n + cols - 1) / cols

	dims := activeList.Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
		return s.layoutRow(gtx, w, activeThumbs, row, cols, n)
	})

	// 5. Trigger next page load (normal mode only).
	if !s.historyOpen {
		s.mu.Lock()
		nearEnd := rows == 0 || s.list.Position.First+s.list.Position.Count+3 >= rows
		canLoad := !s.loading && (s.lastPage == 0 || s.page < s.lastPage)
		s.mu.Unlock()
		if nearEnd && canLoad {
			s.loadNextPage(w)
		}
	}

	// 6. Overlays drawn on top of the grid.
	if s.historyOpen && len(s.historyThumbs) == 0 {
		gtx2 := gtx
		gtx2.Constraints = layout.Exact(gtx.Constraints.Max)
		layout.Center.Layout(gtx2, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(s.theme, unit.Sp(14), "No history yet — set a wallpaper to get started")
			lbl.Color = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
			return lbl.Layout(gtx)
		})
	}
	s.drawStatus(gtx)
	if s.searchOpen {
		s.drawSearch(gtx)
	}
	if s.collectOpen {
		s.drawCollections(gtx)
	}
	if s.lbOpen {
		s.drawLightbox(gtx, w)
	}
	if s.helpOpen {
		s.drawHelp(gtx)
	}

	return dims
}

func (s *state) layoutRow(gtx layout.Context, w *app.Window, thumbs []*Thumb, row, cols, total int) layout.Dimensions {
	children := make([]layout.FlexChild, cols)
	for c := 0; c < cols; c++ {
		idx := row*cols + c
		if idx < total {
			t := thumbs[idx]
			activeSel := s.selected
			if s.historyOpen {
				activeSel = s.historySelected
			}
			sel := idx == activeSel
			children[c] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if t.RightClicked(gtx) {
						s.openLightbox(t, w)
					}
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
	// Enable text-input mode always so key.EditEvent delivers typed characters
	// (needed to capture ? which Gio doesn't deliver as a key.Event on Linux).
	key.InputHintOp{Tag: &s.kt, Hint: key.HintAny}.Add(gtx.Ops)

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
			key.Filter{Focus: &s.kt, Name: key.NameSpace},
			// Grid sizing.
			key.Filter{Focus: &s.kt, Name: "-"},
			key.Filter{Focus: &s.kt, Name: "="},
			key.Filter{Focus: &s.kt, Name: "+", Required: key.ModShift},
			// Sorting — Shift variants.
			key.Filter{Focus: &s.kt, Name: "H", Required: key.ModShift},
			key.Filter{Focus: &s.kt, Name: "L", Required: key.ModShift},
			key.Filter{Focus: &s.kt, Name: "T", Required: key.ModShift},
			key.Filter{Focus: &s.kt, Name: "R", Required: key.ModShift},
			// History toggle.
			key.Filter{Focus: &s.kt, Name: "J", Required: key.ModShift},
			// Search open.
			key.Filter{Focus: &s.kt, Name: "S", Required: key.ModShift},
			key.Filter{Focus: &s.kt, Name: "/"},
			// Collections open.
			key.Filter{Focus: &s.kt, Name: "C", Required: key.ModShift},
			// Lightbox.
			key.Filter{Focus: &s.kt, Name: "P"},
			// Open in browser.
			key.Filter{Focus: &s.kt, Name: "O"},
			// Universal actions.
			key.Filter{Focus: &s.kt, Name: key.NameReturn},
			key.Filter{Focus: &s.kt, Name: key.NameReturn, Required: key.ModShift},
			key.Filter{Focus: &s.kt, Name: key.NameEscape},
			key.Filter{Focus: &s.kt, Name: "Q"},
			// Backspace for search editing.
			key.Filter{Focus: &s.kt, Name: key.NameDeleteBackward},
			// Help overlay — ? is captured via key.EditEvent, not key.Event.
		)
		if !ok {
			break
		}

		switch ev := e.(type) {
		case key.EditEvent:
			// Open help overlay when ? is typed (Gio doesn't deliver Shift+/ as a key.Event on Linux).
			if ev.Text == "?" && !s.searchOpen && !s.collectOpen && !s.lbOpen && !s.helpOpen {
				s.helpOpen = true
				continue
			}
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

			if s.helpOpen {
				switch ev.Name {
				case key.NameEscape, "Q":
					s.helpOpen = false
				}
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

			if s.lbOpen {
				switch ev.Name {
				case "H", key.NameLeftArrow:
					if s.lbTagIdx > 0 {
						s.lbTagIdx--
					} else if s.lbTagIdx == 0 {
						s.lbTagIdx = -1
					}
				case "L", key.NameRightArrow:
					s.mu.Lock()
					nTags := 0
					if s.lbDetail != nil {
						nTags = len(s.lbDetail.Tags)
					}
					s.mu.Unlock()
					if s.lbTagIdx < nTags-1 {
						s.lbTagIdx++
					}
				case "K", key.NameUpArrow:
					s.lbTagIdx = s.lbTagNavVertical(s.lbTagIdx, -1)
				case "J", key.NameDownArrow:
					s.lbTagIdx = s.lbTagNavVertical(s.lbTagIdx, +1)
				case key.NameReturn:
					shift := ev.Modifiers.Contain(key.ModShift)
					s.mu.Lock()
					detail := s.lbDetail
					s.mu.Unlock()
					if !shift && detail != nil && s.lbTagIdx >= 0 && s.lbTagIdx < len(detail.Tags) {
						tagName := detail.Tags[s.lbTagIdx].Name
						s.lbOpen = false
						s.lbTagIdx = -1
						s.applySearch("#"+tagName, w)
					} else if s.lbThumb != nil {
						t := s.lbThumb
						s.lbOpen = false
						if shift {
							t.startDownloadNoClose(w)
						} else {
							t.startDownload(w)
						}
					}
				case "O":
					if s.lbThumb != nil {
						go openInBrowser("https://wallhaven.cc/w/" + s.lbThumb.ID)
					}
				case "P", key.NameEscape, "Q":
					s.lbOpen = false
					s.lbTagIdx = -1
				}
				continue
			}

			// Normal mode.
			shift := ev.Modifiers.Contain(key.ModShift)
			switch {
			// History toggle (Shift+J).
			case ev.Name == "J" && shift:
				s.openHistory(w)
			case ev.Name == "H" && shift:
				s.applySorting("hot", w)
			case ev.Name == "T" && shift:
				s.applySorting("toplist", w)
			case ev.Name == "L" && shift:
				s.applySorting("date_added", w)
			case ev.Name == "R" && shift:
				s.applySorting("random", w)
			case (ev.Name == "S" && shift) || (ev.Name == "/" && !shift):
				s.searchOpen = true
				s.searchText = ""
				searchJustOpened = true
			case ev.Name == "C" && shift:
				s.openCollections(w)
			// Lightbox preview — works in both modes.
			case ev.Name == "P":
				var thumbs []*Thumb
				var sel int
				if s.historyOpen {
					thumbs = s.historyThumbs
					sel = s.historySelected
				} else {
					s.mu.Lock()
					thumbs = s.thumbs
					s.mu.Unlock()
					sel = s.selected
				}
				if sel >= 0 && sel < len(thumbs) {
					s.openLightbox(thumbs[sel], w)
				}
			// Open in browser — works in both modes.
			case ev.Name == "O":
				var thumbs []*Thumb
				var sel int
				if s.historyOpen {
					thumbs = s.historyThumbs
					sel = s.historySelected
				} else {
					s.mu.Lock()
					thumbs = s.thumbs
					s.mu.Unlock()
					sel = s.selected
				}
				if sel >= 0 && sel < len(thumbs) {
					go openInBrowser("https://wallhaven.cc/w/" + thumbs[sel].ID)
				}
			case ev.Name == "Q" || ev.Name == key.NameEscape:
				w.Perform(system.ActionClose)
			// Set wallpaper (keep open) — works in both modes.
			case ev.Name == key.NameReturn && shift:
				var thumbs []*Thumb
				var sel int
				if s.historyOpen {
					thumbs = s.historyThumbs
					sel = s.historySelected
				} else {
					s.mu.Lock()
					thumbs = s.thumbs
					s.mu.Unlock()
					sel = s.selected
				}
				if sel >= 0 && sel < len(thumbs) {
					thumbs[sel].startDownloadNoClose(w)
				}
			case ev.Name == key.NameReturn:
				s.activateSelected(w)
			case ev.Name == "-":
				if s.cfg.ThumbSize > 100 {
					s.cfg.ThumbSize -= 20
				}
			case ev.Name == "=" || ev.Name == "+":
				if s.cfg.ThumbSize < 500 {
					s.cfg.ThumbSize += 20
				}
			case ev.Name == "H" || ev.Name == key.NameLeftArrow:
				s.navigate(-1, 0)
			case ev.Name == "L" || ev.Name == key.NameRightArrow:
				s.navigate(1, 0)
			case ev.Name == "K" || ev.Name == key.NameUpArrow:
				s.navigate(0, -1)
			case ev.Name == "J" || ev.Name == key.NameDownArrow:
				s.navigate(0, 1)
			case ev.Name == key.NameSpace:
				s.pageDown()
			}
		}
	}
}

// navigate moves the keyboard selection by (dx, dy) cells and scrolls the
// list to keep the selected row visible. Works for both the main grid and
// the history view.
func (s *state) navigate(dx, dy int) {
	var n int
	var selPtr *int
	var list *layout.List
	if s.historyOpen {
		n = len(s.historyThumbs)
		selPtr = &s.historySelected
		list = &s.historyList
	} else {
		s.mu.Lock()
		n = len(s.thumbs)
		s.mu.Unlock()
		selPtr = &s.selected
		list = &s.list
	}

	if n == 0 {
		return
	}
	cols := s.cols
	if cols < 1 {
		cols = 1
	}
	if *selPtr < 0 {
		*selPtr = 0
		return
	}

	row := *selPtr / cols
	col := *selPtr % cols
	newCol := col + dx
	newRow := row + dy

	if newCol < 0 || newCol >= cols {
		return
	}
	newIdx := newRow*cols + newCol
	if newIdx < 0 || newIdx >= n {
		return
	}
	*selPtr = newIdx

	selRow := newIdx / cols
	pos := list.Position
	if selRow < pos.First {
		list.ScrollTo(selRow)
	} else if pos.Count > 0 && pos.BeforeEnd && selRow >= pos.First+pos.Count-1 {
		list.Position.First = pos.First + 1
		list.Position.Offset = 0
		list.Position.BeforeEnd = true
	}
}

// pageDown scrolls the grid by one full page (the number of currently visible
// rows) and moves the selection to the first cell of the new page.
func (s *state) pageDown() {
	var n int
	var selPtr *int
	var list *layout.List
	if s.historyOpen {
		n = len(s.historyThumbs)
		selPtr = &s.historySelected
		list = &s.historyList
	} else {
		s.mu.Lock()
		n = len(s.thumbs)
		s.mu.Unlock()
		selPtr = &s.selected
		list = &s.list
	}
	if n == 0 {
		return
	}
	cols := s.cols
	if cols < 1 {
		cols = 1
	}
	pageRows := list.Position.Count
	if pageRows < 1 {
		pageRows = 1
	}
	if *selPtr < 0 {
		*selPtr = 0
	}
	newIdx := *selPtr + pageRows*cols
	if newIdx >= n {
		newIdx = n - 1
	}
	*selPtr = newIdx
	list.ScrollTo(newIdx / cols)
}

// activateSelected downloads and sets the wallpaper for the selected cell.
func (s *state) activateSelected(w *app.Window) {
	var thumbs []*Thumb
	var sel int
	if s.historyOpen {
		thumbs = s.historyThumbs
		sel = s.historySelected
	} else {
		s.mu.Lock()
		thumbs = s.thumbs
		s.mu.Unlock()
		sel = s.selected
	}
	if sel < 0 || sel >= len(thumbs) {
		return
	}
	thumbs[sel].startDownload(w)
}

// applySorting resets the gallery with a new sort mode.
func (s *state) applySorting(sorting string, w *app.Window) {
	s.historyOpen = false
	seed := ""
	if sorting == "random" {
		seed = newSeed()
	}
	s.mu.Lock()
	s.sorting = sorting
	s.seed = seed
	s.srchQ = s.cfg.Query
	s.collLabel = ""
	s.lbOpen = false
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
	s.historyOpen = false
	sorting := s.cfg.SearchSorting
	seed := ""
	if sorting == "random" {
		seed = newSeed()
	}
	s.mu.Lock()
	s.sorting = sorting
	s.seed = seed
	s.srchQ = query
	s.collLabel = ""
	s.lbOpen = false
	s.queryObj = buildQuery(s.cfg, sorting, query, seed)
	s.thumbs = nil
	s.page = 0
	s.lastPage = 0
	s.loading = false
	s.mu.Unlock()
	s.selected = -1
	w.Invalidate()
}

// openHistory loads history entries, creates Thumb objects for them, and
// switches to the history grid view.
func (s *state) openHistory(w *app.Window) {
	entries := loadHistory()
	cfg := s.cfg
	th := s.theme

	thumbs := make([]*Thumb, len(entries))
	for i, e := range entries {
		thumbs[i] = &Thumb{
			ID:       e.ID,
			ThumbURL: e.ThumbURL,
			FullURL:  e.FullURL,
			cfg:      cfg,
			theme:    th,
		}
	}

	s.historyThumbs = thumbs
	s.historySelected = -1
	s.historyList = layout.List{Axis: layout.Vertical}
	s.historyOpen = true

	// Kick off thumbnail loads — all should hit the disk cache instantly.
	for _, t := range thumbs {
		go t.load(w)
	}
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
		th := s.theme
		for _, t := range thumbs {
			t.cfg = cfg
			t.theme = th
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

	var viewLabel, viewSuffix, pageLabel string
	if s.historyOpen {
		viewLabel = "history"
	} else {
		viewLabel = sortingLabel(sorting)
		switch sorting {
		case "relevance":
			if srchQ != "" {
				viewSuffix = ": " + srchQ
			}
		case "collection":
			if collLabel != "" {
				viewSuffix = ": " + collLabel
			}
		}
		if page > 0 {
			pageLabel = fmt.Sprintf(" · %d/%d", page, lastPage)
		}
	}

	gtx2 := gtx
	gtx2.Constraints = layout.Exact(gtx.Constraints.Max)
	layout.SE.Layout(gtx2, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// Record dims first, then draw background, then replay text on top.
			macro := op.Record(gtx.Ops)
			dims := layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.theme, unit.Sp(11), viewLabel)
					lbl.Color = color.NRGBA{R: 230, G: 200, B: 50, A: 255}
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.theme, unit.Sp(11), viewSuffix)
					lbl.Color = color.NRGBA{R: 100, G: 200, B: 100, A: 255}
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.theme, unit.Sp(11), pageLabel)
					lbl.Color = color.NRGBA{R: 122, G: 122, B: 122, A: 255}
					return lbl.Layout(gtx)
				}),
			)
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
		color.NRGBA{A: 210},
		clip.Rect{Max: gtx.Constraints.Max}.Op(),
	)

	boxW := min(gtx.Dp(unit.Dp(360)), gtx.Constraints.Max.X-gtx.Dp(unit.Dp(32)))
	boxH := gtx.Dp(unit.Dp(34))
	boxX := (gtx.Constraints.Max.X - boxW) / 2
	boxY := gtx.Constraints.Max.Y*2/5 - boxH/2

	// White border with rounded corners.
	bw := gtx.Dp(unit.Dp(1))
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 255, G: 255, B: 255, A: 255},
		clip.RRect{
			Rect: image.Rect(boxX-bw, boxY-bw, boxX+boxW+bw, boxY+boxH+bw),
			NE: 4, NW: 4, SE: 4, SW: 4,
		}.Op(gtx.Ops),
	)

	// Box background — pure black.
	paint.FillShape(gtx.Ops,
		color.NRGBA{A: 255},
		clip.RRect{
			Rect: image.Rect(boxX, boxY, boxX+boxW, boxY+boxH),
			NE: 4, NW: 4, SE: 4, SW: 4,
		}.Op(gtx.Ops),
	)

	// Search text + cursor, or placeholder.
	textPad := gtx.Dp(unit.Dp(10))
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
		textColor = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}
	lbl := material.Label(s.theme, unit.Sp(12), displayText)
	lbl.Color = textColor
	layout.Center.Layout(textGtx, lbl.Layout)
	off.Pop()

	// Hint text below the box.
	hintY := boxY + boxH + gtx.Dp(unit.Dp(8))
	hintH := gtx.Dp(unit.Dp(20))
	paint.FillShape(gtx.Ops,
		color.NRGBA{A: 200},
		clip.Rect{Min: image.Pt(boxX, hintY), Max: image.Pt(boxX+boxW, hintY+hintH)}.Op(),
	)
	hintOff := op.Offset(image.Pt(boxX, hintY)).Push(gtx.Ops)
	hintGtx := gtx
	hintGtx.Constraints = layout.Exact(image.Pt(boxW, hintH))
	hintLbl := material.Label(s.theme, unit.Sp(10), "Enter to search  ·  Esc to cancel")
	hintLbl.Color = color.NRGBA{R: 180, G: 180, B: 180, A: 255}
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
	s.historyOpen = false
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
		color.NRGBA{A: 210},
		clip.Rect{Max: gtx.Constraints.Max}.Op(),
	)

	s.mu.Lock()
	colls := s.collections
	collSel := s.collSelected
	loading := s.collLoading
	s.mu.Unlock()

	rowH := gtx.Dp(unit.Dp(24))
	boxW := min(gtx.Dp(unit.Dp(360)), gtx.Constraints.Max.X-gtx.Dp(unit.Dp(32)))
	boxH := rowH * max(len(colls), 1)
	boxX := (gtx.Constraints.Max.X - boxW) / 2
	boxY := gtx.Constraints.Max.Y*2/5 - boxH/2

	// White border with rounded corners.
	bw := gtx.Dp(unit.Dp(1))
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 255, G: 255, B: 255, A: 255},
		clip.RRect{
			Rect: image.Rect(boxX-bw, boxY-bw, boxX+boxW+bw, boxY+boxH+bw),
			NE: 4, NW: 4, SE: 4, SW: 4,
		}.Op(gtx.Ops),
	)

	// Box background — pure black.
	paint.FillShape(gtx.Ops,
		color.NRGBA{A: 255},
		clip.RRect{
			Rect: image.Rect(boxX, boxY, boxX+boxW, boxY+boxH),
			NE: 4, NW: 4, SE: 4, SW: 4,
		}.Op(gtx.Ops),
	)

	textPad := gtx.Dp(unit.Dp(10))

	if loading || len(colls) == 0 {
		msg := "Loading collections…"
		if !loading {
			msg = "No collections found"
		}
		off := op.Offset(image.Pt(boxX+textPad, boxY)).Push(gtx.Ops)
		tGtx := gtx
		tGtx.Constraints = layout.Exact(image.Pt(boxW-2*textPad, rowH))
		lbl := material.Label(s.theme, unit.Sp(12), msg)
		lbl.Color = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
		layout.W.Layout(tGtx, lbl.Layout)
		off.Pop()
	} else {
		for i, coll := range colls {
			y := boxY + i*rowH
			// Highlight selected row.
			if i == collSel {
				paint.FillShape(gtx.Ops,
					color.NRGBA{R: 255, G: 255, B: 255, A: 30},
					clip.Rect{Min: image.Pt(boxX, y), Max: image.Pt(boxX+boxW, y+rowH)}.Op(),
				)
				// Left accent bar.
				paint.FillShape(gtx.Ops,
					color.NRGBA{R: 255, G: 255, B: 255, A: 200},
					clip.Rect{Min: image.Pt(boxX, y), Max: image.Pt(boxX+gtx.Dp(unit.Dp(2)), y+rowH)}.Op(),
				)
			}
			off := op.Offset(image.Pt(boxX+textPad, y+gtx.Dp(unit.Dp(4)))).Push(gtx.Ops)
			tGtx := gtx
			tGtx.Constraints = layout.Exact(image.Pt(boxW-2*textPad, rowH))

			label := fmt.Sprintf("%s  (%d)", coll.Label, coll.Count)
			textColor := color.NRGBA{R: 150, G: 150, B: 150, A: 255}
			if i == collSel {
				textColor = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			}
			lbl := material.Label(s.theme, unit.Sp(12), label)
			lbl.Color = textColor
			layout.W.Layout(tGtx, lbl.Layout)
			off.Pop()
		}
	}

	// Hint text below the box.
	hintY := boxY + boxH + gtx.Dp(unit.Dp(8))
	hintH := gtx.Dp(unit.Dp(20))
	paint.FillShape(gtx.Ops,
		color.NRGBA{A: 200},
		clip.Rect{Min: image.Pt(boxX, hintY), Max: image.Pt(boxX+boxW, hintY+hintH)}.Op(),
	)
	hintOff := op.Offset(image.Pt(boxX, hintY)).Push(gtx.Ops)
	hintGtx := gtx
	hintGtx.Constraints = layout.Exact(image.Pt(boxW, hintH))
	hintLbl := material.Label(s.theme, unit.Sp(10), "j/k to navigate  ·  Enter to open  ·  Esc to cancel")
	hintLbl.Color = color.NRGBA{R: 180, G: 180, B: 180, A: 255}
	layout.Center.Layout(hintGtx, hintLbl.Layout)
	hintOff.Pop()
}

// lbTagNavVertical moves the tag selection up (dir=-1) or down (dir=+1)
// within the wrapped tag grid, picking the nearest chip by x-center on the adjacent row.
func (s *state) lbTagNavVertical(idx, dir int) int {
	chips := s.lbTagChips
	if len(chips) == 0 {
		return idx
	}

	// Find current chip's row y and center x.
	curY := -1
	curCX := 0
	if idx >= 0 && idx < len(chips) {
		curY = chips[idx].y
		curCX = chips[idx].x + chips[idx].w/2
	} else {
		// Nothing selected: enter from top or bottom.
		if dir > 0 {
			return 0
		}
		return len(chips) - 1
	}

	// Find the target row y.
	targetY := -1
	for i := range chips {
		ry := chips[i].y
		if dir < 0 && ry < curY && (targetY == -1 || ry > targetY) {
			targetY = ry
		} else if dir > 0 && ry > curY && (targetY == -1 || ry < targetY) {
			targetY = ry
		}
	}
	if targetY == -1 {
		return idx // already at top/bottom row
	}

	// Pick chip on targetY whose center x is closest.
	best := -1
	bestDist := 1<<31 - 1
	for i, c := range chips {
		if c.y != targetY {
			continue
		}
		cx := c.x + c.w/2
		d := cx - curCX
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	if best >= 0 {
		return best
	}
	return idx
}

// drawHelp renders the keyboard shortcuts overlay.
func (s *state) drawHelp(gtx layout.Context) {
	// Semi-transparent backdrop.
	paint.FillShape(gtx.Ops,
		color.NRGBA{A: 160},
		clip.Rect{Max: gtx.Constraints.Max}.Op(),
	)

	type entry struct{ key, desc string }
	type section struct {
		title   string
		entries []entry
	}

	leftSections := []section{
		{
			"NAVIGATION",
			[]entry{
				{"h/j/k/l  ·  arrows", "Move selection"},
			},
		},
		{
			"ACTIONS",
			[]entry{
				{"Enter", "Set as wallpaper"},
			{"Shift+Enter", "Set wallpaper (keep open)"},
				{"p", "Preview (lightbox)"},
				{"o", "Open in browser"},
				{"s  ·  /", "Search"},
				{"Shift+C", "Collections"},
				{"Shift+J", "History"},
				{"-  ·  =", "Zoom out / in"},
				{"q  ·  Esc", "Quit"},
			},
		},
		{
			"SORTING",
			[]entry{
				{"Shift+H", "Hot"},
				{"Shift+T", "Toplist"},
				{"Shift+L", "Latest"},
				{"Shift+R", "Random"},
			},
		},
	}

	rightSections := []section{
		{
			"LIGHTBOX",
			[]entry{
				{"h/l  ·  arrows", "Navigate tags"},
				{"j/k  ·  arrows", "Prev / next row"},
				{"Enter", "Set wallpaper or search tag"},
				{"Shift+Enter", "Set wallpaper (keep open)"},
				{"o", "Open in browser"},
				{"p  ·  Esc", "Close"},
			},
		},
		{
			"SEARCH",
			[]entry{
				{"Enter", "Submit search"},
				{"Esc", "Cancel"},
			},
		},
		{
			"COLLECTIONS",
			[]entry{
				{"j/k", "Navigate"},
				{"Enter", "Open collection"},
				{"Esc", "Cancel"},
			},
		},
	}

	rowH := gtx.Dp(unit.Dp(22))
	sectionGap := gtx.Dp(unit.Dp(12))
	sectionHdrH := gtx.Dp(unit.Dp(24))
	titleH := gtx.Dp(unit.Dp(34))
	pad := gtx.Dp(unit.Dp(20))
	colGap := gtx.Dp(unit.Dp(24))

	colH := func(sects []section) int {
		h := 0
		for i, sec := range sects {
			if i > 0 {
				h += sectionGap
			}
			h += sectionHdrH
			h += len(sec.entries) * rowH
		}
		return h
	}

	contentH := colH(leftSections)
	if rh := colH(rightSections); rh > contentH {
		contentH = rh
	}

	boxW := min(gtx.Dp(unit.Dp(660)), gtx.Constraints.Max.X-gtx.Dp(unit.Dp(32)))
	colW := (boxW - pad*2 - colGap) / 2
	boxH := pad + titleH + gtx.Dp(unit.Dp(12)) + contentH + pad
	boxX := (gtx.Constraints.Max.X - boxW) / 2
	boxY := (gtx.Constraints.Max.Y - boxH) / 2
	if boxY < gtx.Dp(unit.Dp(16)) {
		boxY = gtx.Dp(unit.Dp(16))
	}

	// Box background.
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 22, G: 22, B: 28, A: 255},
		clip.RRect{
			Rect: image.Rect(boxX, boxY, boxX+boxW, boxY+boxH),
			NE: 12, NW: 12, SE: 12, SW: 12,
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

	// Title.
	{
		off := op.Offset(image.Pt(boxX+pad, boxY+pad)).Push(gtx.Ops)
		tGtx := gtx
		tGtx.Constraints = layout.Exact(image.Pt(boxW-2*pad, titleH))
		lbl := material.Label(s.theme, unit.Sp(16), "Keyboard Shortcuts")
		lbl.Color = color.NRGBA{R: 232, G: 232, B: 232, A: 255}
		layout.W.Layout(tGtx, lbl.Layout)
		off.Pop()
	}

	keyColor := color.NRGBA{R: 186, G: 172, B: 255, A: 255}
	descColor := color.NRGBA{R: 150, G: 150, B: 150, A: 255}
	hdrColor := color.NRGBA{R: 90, G: 90, B: 100, A: 255}

	drawSections := func(sects []section, startX, startY int) {
		y := startY
		for i, sec := range sects {
			if i > 0 {
				y += sectionGap
			}
			// Section header.
			{
				off := op.Offset(image.Pt(startX, y)).Push(gtx.Ops)
				tGtx := gtx
				tGtx.Constraints = layout.Exact(image.Pt(colW, sectionHdrH))
				lbl := material.Label(s.theme, unit.Sp(10), sec.title)
				lbl.Color = hdrColor
				layout.W.Layout(tGtx, lbl.Layout)
				off.Pop()
			}
			y += sectionHdrH
			// Entries.
			keyW := colW * 2 / 5
			for _, e := range sec.entries {
				kOff := op.Offset(image.Pt(startX, y)).Push(gtx.Ops)
				kGtx := gtx
				kGtx.Constraints = layout.Exact(image.Pt(keyW, rowH))
				kLbl := material.Label(s.theme, unit.Sp(12), e.key)
				kLbl.Color = keyColor
				layout.W.Layout(kGtx, kLbl.Layout)
				kOff.Pop()

				dOff := op.Offset(image.Pt(startX+keyW, y)).Push(gtx.Ops)
				dGtx := gtx
				dGtx.Constraints = layout.Exact(image.Pt(colW-keyW, rowH))
				dLbl := material.Label(s.theme, unit.Sp(12), e.desc)
				dLbl.Color = descColor
				layout.W.Layout(dGtx, dLbl.Layout)
				dOff.Pop()

				y += rowH
			}
		}
	}

	contentY := boxY + pad + titleH + gtx.Dp(unit.Dp(12))
	drawSections(leftSections, boxX+pad, contentY)
	drawSections(rightSections, boxX+pad+colW+colGap, contentY)

	// Hint below the box.
	hintOff := op.Offset(image.Pt(boxX, boxY+boxH+gtx.Dp(unit.Dp(8)))).Push(gtx.Ops)
	hintGtx := gtx
	hintGtx.Constraints = layout.Exact(image.Pt(boxW, gtx.Dp(unit.Dp(20))))
	hintLbl := material.Label(s.theme, unit.Sp(11), "?  or  Esc to close")
	hintLbl.Color = color.NRGBA{R: 80, G: 80, B: 80, A: 200}
	layout.Center.Layout(hintGtx, hintLbl.Layout)
	hintOff.Pop()
}
