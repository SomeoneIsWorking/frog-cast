package main

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestSheetCloses(t *testing.T) {
	test.NewTempApp(t)
	w := test.NewWindow(nil)
	w.Resize(fyne.NewSize(390, 844))
	u := &ui{window: w}
	c := w.Canvas()
	parts := func(sh *sheet) (scrim, panel *tapSink) {
		stack := sh.overlay.(*fyne.Container)
		scrim = stack.Objects[0].(*tapSink)
		border := stack.Objects[1].(*fyne.Container)
		for _, o := range border.Objects {
			if p, ok := o.(*fyne.Container); ok && len(p.Objects) == 1 {
				if ts, ok := p.Objects[0].(*tapSink); ok {
					panel = ts
				}
			}
		}
		return scrim, panel
	}

	closed := 0
	sh := u.showSheet("Title", widget.NewLabel("body"))
	sh.onClosed = func() { closed++ }
	scrim, panel := parts(sh)
	test.Tap(panel)
	if c.Overlays().Top() != sh.overlay {
		t.Fatal("a tap on the panel closed the sheet")
	}
	test.Tap(scrim)
	if c.Overlays().Top() != nil || closed != 1 {
		t.Fatalf("scrim tap: overlay %v, onClosed ran %d times", c.Overlays().Top(), closed)
	}

	var keys []fyne.KeyName
	c.SetOnTypedKey(func(e *fyne.KeyEvent) { keys = append(keys, e.Name) })
	sh = u.showSheet("Title", widget.NewLabel("body"))
	c.OnTypedKey()(&fyne.KeyEvent{Name: fyne.KeyA})
	c.OnTypedKey()(&fyne.KeyEvent{Name: mobile.KeyBack})
	if c.Overlays().Top() != nil {
		t.Fatal("Back left the sheet open")
	}
	c.OnTypedKey()(&fyne.KeyEvent{Name: fyne.KeyB})
	if len(keys) != 2 || keys[0] != fyne.KeyA || keys[1] != fyne.KeyB {
		t.Fatalf("keys reaching the screen: %v", keys)
	}

	ran := false
	u.confirm("Clear?", "Sure?", "Clear", widget.DangerImportance, func() { ran = true })
	top := c.Overlays().Top().(*fyne.Container)
	var clear *widget.Button
	var walk func(o fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		switch v := o.(type) {
		case *widget.Button:
			if v.Text == "Clear" {
				clear = v
			}
		case *fyne.Container:
			for _, ch := range v.Objects {
				walk(ch)
			}
		case *tapSink:
			walk(v.content)
		}
	}
	walk(top)
	if clear == nil {
		t.Fatal("no Clear button")
	}
	test.Tap(clear)
	if !ran || c.Overlays().Top() != nil {
		t.Fatalf("Clear: ran %v, overlay %v", ran, c.Overlays().Top())
	}
}
