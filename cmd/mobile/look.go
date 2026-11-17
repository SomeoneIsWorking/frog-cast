package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// card puts content on a rounded surface.
func card(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(colorSurface)
	bg.CornerRadius = cardRadius
	return container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(12, 12, 14, 14), content))
}

// compactCard is a card with the tighter padding of a list row.
func compactCard(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(colorSurface)
	bg.CornerRadius = cardRadius
	return container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(6, 6, 10, 8), content))
}

// tapCard is a card that runs onTap when tapped, outside a list.
type tapCard struct {
	widget.BaseWidget
	content fyne.CanvasObject
	onTap   func()
}

func newTapCard(content fyne.CanvasObject, onTap func()) *tapCard {
	t := &tapCard{content: content, onTap: onTap}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tapCard) Tapped(*fyne.PointEvent) { t.onTap() }

func (t *tapCard) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.content)
}

// badge is an icon on a tinted rounded square, leading a row.
func badge(icon fyne.Resource) (fyne.CanvasObject, *widget.Icon) {
	bg := canvas.NewRectangle(colorAccentSoft)
	bg.CornerRadius = 10
	bg.SetMinSize(fyne.NewSize(40, 40))
	i := widget.NewIcon(theme.NewPrimaryThemedResource(icon))
	return container.NewStack(bg, container.NewCenter(i)), i
}

// row is a list row: a badge, a title, a muted line of detail and a chevron,
// on a card.
type row struct {
	widget.BaseWidget
	icon    *widget.Icon
	title   *widget.Label
	detail  *widget.Label
	trail   fyne.CanvasObject
	content fyne.CanvasObject
}

func newRow(icon fyne.Resource) *row {
	return newRowWith(icon, widget.NewIcon(theme.NewColoredResource(theme.NavigateNextIcon(), theme.ColorNamePlaceHolder)))
}

// newRowWith is a row ending in trail instead of a chevron.
func newRowWith(icon fyne.Resource, trail fyne.CanvasObject) *row {
	r := &row{trail: trail}
	b, i := badge(icon)
	r.icon = i
	r.title = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	r.title.Truncation = fyne.TextTruncateEllipsis
	r.detail = widget.NewLabel("")
	r.detail.Importance = widget.LowImportance
	r.detail.SizeName = theme.SizeNameCaptionText
	r.detail.Truncation = fyne.TextTruncateEllipsis
	texts := container.New(layout.NewCustomPaddedVBoxLayout(-18), r.title, r.detail)
	body := container.NewBorder(nil, nil, container.NewCenter(b), container.NewCenter(trail), container.NewVBox(layout.NewSpacer(), texts, layout.NewSpacer()))
	r.content = container.New(layout.NewCustomPaddedLayout(3, 3, 0, 0), compactCard(body))
	r.ExtendBaseWidget(r)
	return r
}

func (r *row) set(title, detail string) {
	r.title.SetText(title)
	r.detail.SetText(detail)
	r.detail.Hidden = detail == ""
}

func (r *row) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.content)
}

// cardList is a list of rows with no separators: the cards carry the gaps.
func cardList(length func() int, create func() fyne.CanvasObject, update func(widget.ListItemID, fyne.CanvasObject)) *widget.List {
	l := widget.NewList(length, create, update)
	l.HideSeparators = true
	return l
}

// thinBar is a slim progress bar: an accent fill over a raised track.
type thinBar struct {
	widget.BaseWidget
	value float64
	track *canvas.Rectangle
	fill  *canvas.Rectangle
}

func newThinBar() *thinBar {
	b := &thinBar{track: canvas.NewRectangle(colorRaised), fill: canvas.NewRectangle(colorAccent)}
	b.track.CornerRadius = 3
	b.fill.CornerRadius = 3
	b.ExtendBaseWidget(b)
	return b
}

// SetValue sets the filled share, 0 to 1.
func (b *thinBar) SetValue(v float64) {
	b.value = min(max(v, 0), 1)
	b.Refresh()
}

func (b *thinBar) CreateRenderer() fyne.WidgetRenderer {
	return &thinBarRenderer{b: b}
}

type thinBarRenderer struct{ b *thinBar }

func (r *thinBarRenderer) Layout(size fyne.Size) {
	r.b.track.Resize(size)
	r.b.fill.Resize(fyne.NewSize(size.Width*float32(r.b.value), size.Height))
}

func (r *thinBarRenderer) MinSize() fyne.Size { return fyne.NewSize(40, 6) }

func (r *thinBarRenderer) Refresh() {
	r.Layout(r.b.Size())
	canvas.Refresh(r.b)
}

func (r *thinBarRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.b.track, r.b.fill}
}

func (r *thinBarRenderer) Destroy() {}

// heading1 is a screen title.
func heading1(text string) *widget.Label {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.SizeName = theme.SizeNameHeadingText
	return l
}

// section is a small title above a group.
func section(text string) *widget.Label {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.Importance = widget.LowImportance
	l.SizeName = theme.SizeNameCaptionText
	return l
}

// caption is secondary text under a title or row, wrapped to its width.
func caption(text string) *widget.Label {
	l := note(text)
	l.Wrapping = fyne.TextWrapWord
	return l
}

// note is a one-line secondary text, for places with no width to wrap to
// (a centered message).
func note(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Importance = widget.LowImportance
	return l
}

// emptyState is a centered icon, title and hint for a screen with nothing
// to show.
func emptyState(icon fyne.Resource, title, hint string) fyne.CanvasObject {
	bg := canvas.NewCircle(colorAccentSoft)
	i := widget.NewIcon(theme.NewPrimaryThemedResource(icon))
	mark := container.NewGridWrap(fyne.NewSize(72, 72), container.NewStack(bg, container.NewPadded(container.NewPadded(i))))
	t := widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	t.SizeName = theme.SizeNameSubHeadingText
	h := note(hint)
	h.Alignment = fyne.TextAlignCenter
	h.Wrapping = fyne.TextWrapWord
	// A full-width column, so the hint wraps instead of running off screen.
	return container.NewVBox(layout.NewSpacer(), container.NewCenter(mark), t, h, layout.NewSpacer())
}

// navTabs is the bottom navigation: one page per item, the chosen item
// marked with an accent pill.
type navTabs struct {
	content    fyne.CanvasObject
	body       *fyne.Container
	pages      []fyne.CanvasObject
	items      []*navItem
	selected   int
	OnSelected func(int)
}

type navTab struct {
	label string
	icon  fyne.Resource
	page  fyne.CanvasObject
}

func newNavTabs(tabs ...navTab) *navTabs {
	n := &navTabs{body: container.NewStack()}
	bar := container.NewGridWithColumns(len(tabs))
	for i, t := range tabs {
		item := newNavItem(t.label, t.icon, func() { n.Select(i) })
		n.items = append(n.items, item)
		n.pages = append(n.pages, t.page)
		bar.Add(item)
	}
	barBg := canvas.NewRectangle(colorSurface)
	line := canvas.NewRectangle(colorBorder)
	line.SetMinSize(fyne.NewSize(1, 1))
	barView := container.NewStack(barBg, container.NewBorder(line, nil, nil, nil,
		container.New(layout.NewCustomPaddedLayout(6, 6, 0, 0), bar)))
	n.content = container.NewBorder(nil, barView, nil, nil,
		container.New(layout.NewCustomPaddedLayout(0, 4, 10, 10), n.body))
	n.Select(0)
	return n
}

func (n *navTabs) Selected() int { return n.selected }

func (n *navTabs) Select(i int) {
	n.selected = i
	n.body.Objects = []fyne.CanvasObject{n.pages[i]}
	n.body.Refresh()
	for j, item := range n.items {
		item.setActive(j == i)
	}
	if n.OnSelected != nil {
		n.OnSelected(i)
	}
}

// navItem is one tappable icon and label in the bar.
type navItem struct {
	widget.BaseWidget
	res   fyne.Resource
	icon  *widget.Icon
	pill  *canvas.Rectangle
	label *canvas.Text
	onTap func()
}

func newNavItem(label string, icon fyne.Resource, onTap func()) *navItem {
	n := &navItem{res: icon, onTap: onTap}
	n.icon = widget.NewIcon(icon)
	n.pill = canvas.NewRectangle(color.Transparent)
	n.pill.CornerRadius = 15
	n.pill.SetMinSize(fyne.NewSize(56, 30))
	n.label = canvas.NewText(label, colorMuted)
	n.label.TextSize = 12
	n.label.Alignment = fyne.TextAlignCenter
	n.ExtendBaseWidget(n)
	return n
}

func (n *navItem) setActive(on bool) {
	if on {
		n.icon.SetResource(theme.NewColoredResource(n.res, theme.ColorNamePrimary))
		n.pill.FillColor = colorAccentSoft
		n.label.Color = colorText
		n.label.TextStyle.Bold = true
	} else {
		n.icon.SetResource(theme.NewColoredResource(n.res, theme.ColorNameDisabled))
		n.pill.FillColor = color.Transparent
		n.label.Color = colorMuted
		n.label.TextStyle.Bold = false
	}
	n.pill.Refresh()
	n.label.Refresh()
}

func (n *navItem) Tapped(*fyne.PointEvent) { n.onTap() }

func (n *navItem) CreateRenderer() fyne.WidgetRenderer {
	mark := container.NewCenter(container.NewStack(n.pill, container.NewCenter(n.icon)))
	return widget.NewSimpleRenderer(container.New(layout.NewCustomPaddedVBoxLayout(4), mark, n.label))
}
