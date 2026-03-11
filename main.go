package main

import (
	"image"
	"image/color"
	"log"
	"os"
	"sync"

	wh "github.com/davenicholson-xyz/go-wallhaven/wallhavenapi"
	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func main() {
	go func() {
		w := new(app.Window)
		w.Option(
			app.Title("GoVista"),
		)
		if err := run(w); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

type state struct {
	mu       sync.Mutex
	thumbs   []*Thumb
	list     layout.List
	query    *wh.Query
	page     int
	lastPage int
	loading  bool
}

func run(w *app.Window) error {
	s := &state{
		list:  layout.List{Axis: layout.Vertical},
		query: newQuery(),
	}

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

const maxCellDp = unit.Dp(200)

func (s *state) layout(gtx layout.Context, w *app.Window) layout.Dimensions {
	// Dark background filling the whole window.
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 18, G: 18, B: 18, A: 255},
		clip.Rect{Max: gtx.Constraints.Max}.Op(),
	)

	s.mu.Lock()
	thumbs := s.thumbs
	s.mu.Unlock()

	maxCellPx := gtx.Dp(maxCellDp)
	cols := gtx.Constraints.Max.X / maxCellPx
	if cols < 1 {
		cols = 1
	}

	n := len(thumbs)
	rows := (n + cols - 1) / cols

	dims := s.list.Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
		return layoutRow(gtx, w, thumbs, row, cols, n)
	})

	// Trigger next page when within 3 rows of the end (covers initial fill and scroll-to-bottom).
	s.mu.Lock()
	nearEnd := rows == 0 || s.list.Position.First+s.list.Position.Count+3 >= rows
	canLoad := !s.loading && (s.lastPage == 0 || s.page < s.lastPage)
	s.mu.Unlock()

	if nearEnd && canLoad {
		s.loadNextPage(w)
	}

	return dims
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
	q := s.query
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

func layoutRow(gtx layout.Context, w *app.Window, thumbs []*Thumb, row, cols, total int) layout.Dimensions {
	children := make([]layout.FlexChild, cols)
	for c := 0; c < cols; c++ {
		idx := row*cols + c
		if idx < total {
			t := thumbs[idx]
			children[c] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return t.layout(gtx, w)
				})
			})
		} else {
			// Empty filler cell to keep the row width consistent.
			children[c] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				w := gtx.Constraints.Max.X
				return layout.Dimensions{Size: image.Pt(w, w*9/16)}
			})
		}
	}
	return layout.Flex{}.Layout(gtx, children...)
}
