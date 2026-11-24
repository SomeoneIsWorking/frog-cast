package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// sheetAction is a button along a sheet's bottom; tapping it closes the sheet.
type sheetAction struct {
	label      string
	importance widget.Importance
	tapped     func()
}

// sheet is the app's modal: a card along the bottom of a dimmed screen,
// closed by its buttons, by a tap outside it or by Back.
type sheet struct {
	canvas   fyne.Canvas
	overlay  fyne.CanvasObject
	prevKey  func(*fyne.KeyEvent)
	closed   bool
	onClosed func()
}

var sheetScrim = color.NRGBA{A: 0x99}

// showSheet opens a sheet titled titleText over the window.
func (u *ui) showSheet(titleText string, body fyne.CanvasObject, actions ...sheetAction) *sheet {
	c := u.window.Canvas()
	sh := &sheet{canvas: c}

	title := heading1(titleText)
	title.Wrapping = fyne.TextWrapWord
	handle := canvas.NewRectangle(colorAccentSoft)
	handle.CornerRadius = 2
	head := container.NewVBox(
		container.NewCenter(container.NewGridWrap(fyne.NewSize(40, 4), handle)),
		title,
	)
	var buttons []fyne.CanvasObject
	for _, a := range actions {
		b := widget.NewButton(a.label, func() {
			sh.close()
			if a.tapped != nil {
				a.tapped()
			}
		})
		b.Importance = a.importance
		buttons = append(buttons, b)
	}
	parts := []fyne.CanvasObject{head, body}
	if len(buttons) > 0 {
		parts = append(parts, container.NewGridWithColumns(len(buttons), buttons...))
	}
	panel := newTapSink(card(container.NewVBox(parts...)), nil)

	sh.overlay = container.NewStack(
		newTapSink(canvas.NewRectangle(sheetScrim), sh.close),
		container.NewBorder(nil, container.NewPadded(panel), nil, nil, layout.NewSpacer()),
	)
	sh.prevKey = c.OnTypedKey()
	c.SetOnTypedKey(func(e *fyne.KeyEvent) {
		if e.Name == mobile.KeyBack || e.Name == fyne.KeyEscape {
			sh.close()
			return
		}
		if sh.prevKey != nil {
			sh.prevKey(e)
		}
	})
	c.Overlays().Add(sh.overlay)
	sh.overlay.Resize(c.Size())
	// Wrapped text knows its height only once it has a width.
	sh.overlay.Refresh()
	return sh
}

func (sh *sheet) close() {
	if sh.closed {
		return
	}
	sh.closed = true
	sh.canvas.Overlays().Remove(sh.overlay)
	sh.canvas.SetOnTypedKey(sh.prevKey)
	if sh.onClosed != nil {
		sh.onClosed()
	}
}

// sheetFields stacks labelled controls, each label above its control, the
// way a narrow screen reads them.
func sheetFields(items ...*widget.FormItem) fyne.CanvasObject {
	box := container.NewVBox()
	for _, it := range items {
		box.Add(container.New(layout.NewCustomPaddedVBoxLayout(2), section(it.Text), it.Widget))
	}
	return box
}

// confirm asks before doing something that can't be undone.
func (u *ui) confirm(title, message, action string, importance widget.Importance, yes func()) {
	u.showSheet(title, caption(message),
		sheetAction{label: "Cancel", importance: widget.LowImportance},
		sheetAction{label: action, importance: importance, tapped: yes},
	)
}

// inform shows a message.
func (u *ui) inform(title, message string) {
	u.showSheet(title, caption(message), sheetAction{label: "OK", importance: widget.HighImportance})
}

// tapSink takes the taps on its content: the scrim closes the sheet with
// them, and the panel keeps them from reaching the scrim.
type tapSink struct {
	widget.BaseWidget
	content fyne.CanvasObject
	tapped  func()
}

func newTapSink(content fyne.CanvasObject, tapped func()) *tapSink {
	t := &tapSink{content: content, tapped: tapped}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tapSink) Tapped(*fyne.PointEvent) {
	if t.tapped != nil {
		t.tapped()
	}
}

func (t *tapSink) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.content)
}
