package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"wails-cast/pkg/castapi"
	"wails-cast/pkg/options"
)

// ---------------------------------------------------------------------------
// Now Playing screen
// ---------------------------------------------------------------------------

func (u *ui) showNowPlaying() {
	title := widget.NewLabel("")
	pos := widget.NewLabel("0:00 / 0:00")

	seek := widget.NewSlider(0, 1)
	seek.Step = 1
	var seekingDuration float64
	seek.OnChangeEnded = func(v float64) {
		if seekingDuration <= 0 {
			return
		}
		go func() {
			if _, err := u.client.Control("seek", v); err != nil {
				fyne.Do(func() { u.fail(err) })
			}
		}()
	}

	var currentPos float64

	playPause := widget.NewButton("Pause", nil)
	stop := widget.NewButton("Stop", func() {
		go func() {
			if _, err := u.client.Control("stop", 0); err != nil {
				fyne.Do(func() { u.fail(err) })
			}
		}()
	})
	back30 := widget.NewButton("-30", func() { u.seekRelative(currentPos, -30) })
	back10 := widget.NewButton("-10", func() { u.seekRelative(currentPos, -10) })
	fwd10 := widget.NewButton("+10", func() { u.seekRelative(currentPos, 10) })
	fwd30 := widget.NewButton("+30", func() { u.seekRelative(currentPos, 30) })

	vol := widget.NewSlider(0, 1)
	vol.Step = 0.01
	vol.OnChangeEnded = func(v float64) {
		go func() {
			if _, err := u.client.Control("volume", v); err != nil {
				fyne.Do(func() { u.fail(err) })
			}
		}()
	}

	muted := false
	mute := widget.NewButton("Mute", nil)
	mute.OnTapped = func() {
		action := "mute"
		if muted {
			action = "unmute"
		}
		muted = !muted
		if muted {
			mute.SetText("Unmute")
		} else {
			mute.SetText("Mute")
		}
		go func() {
			if _, err := u.client.Control(action, 0); err != nil {
				fyne.Do(func() { u.fail(err) })
			}
		}()
	}

	subtitle := widget.NewButton("Subtitle", func() { u.showSubtitleDialog() })
	tracks := widget.NewButton("Tracks", func() { u.showTracksDialog() })
	back := widget.NewButton("Back to library", func() {
		u.stopNowPlayingPoll()
		u.window.SetContent(u.tabs)
		if u.tabs != nil {
			u.tabs.SelectIndex(0)
		}
	})

	transport := container.NewGridWithColumns(4, back30, back10, fwd10, fwd30)
	controls := container.NewGridWithColumns(2, playPause, stop)

	content := container.NewVBox(
		back,
		title,
		pos,
		seek,
		transport,
		controls,
		widget.NewLabel("Volume"), vol,
		mute,
		subtitle,
		tracks,
	)
	u.window.SetContent(content)

	pauseState := false
	playPause.OnTapped = func() {
		action := "pause"
		if pauseState {
			action = "resume"
		}
		go func() {
			if _, err := u.client.Control(action, 0); err != nil {
				fyne.Do(func() { u.fail(err) })
			}
		}()
	}

	u.stopNowPlayingPoll()
	u.nowPlayingStop = make(chan struct{})
	stopCh := u.nowPlayingStop
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				st, err := u.client.State()
				if err != nil {
					continue
				}
				fyne.Do(func() {
					title.SetText(st.MediaName)
					currentPos = st.CurrentTime
					seekingDuration = st.Duration
					if st.Duration > 0 {
						seek.Max = st.Duration
						seek.Value = st.CurrentTime
						seek.Refresh()
					}
					pos.SetText(fmt.Sprintf("%s / %s", fmtSec(st.CurrentTime), fmtSec(st.Duration)))
					pauseState = st.Status == "PAUSED"
					if pauseState {
						playPause.SetText("Resume")
					} else {
						playPause.SetText("Pause")
					}
					if !muted && st.Muted {
						muted = true
						mute.SetText("Unmute")
					}
					vol.Value = st.Volume
					vol.Refresh()
				})
			}
		}
	}()

}

// showSubtitleDialog edits the running cast's subtitle style, seeded from
// what the casting instance reports.
func (u *ui) showSubtitleDialog() {
	go func() {
		opts := u.currentSubtitleOpts
		if opts.Path == "" {
			opts.Path = u.currentSubtitlePath
		}
		if st, err := u.client.State(); err == nil && st.Selection != nil {
			sel := st.Selection
			opts = options.SubtitleCastOptions{
				Path:                 sel.SubtitlePath,
				BurnIn:               sel.BurnIn,
				FontSize:             sel.FontSize,
				IgnoreClosedCaptions: sel.IgnoreClosedCaptions,
				DelaySeconds:         sel.DelaySeconds,
				Bold:                 sel.Bold,
				Italic:               sel.Italic,
			}
		}
		if opts.FontSize == 0 {
			opts.FontSize = 24
		}
		fyne.Do(func() { u.showSubtitleForm(opts) })
	}()
}

func (u *ui) showSubtitleForm(opts options.SubtitleCastOptions) {

	delay := widget.NewSlider(-30, 30)
	delay.Step = 0.5
	delay.Value = opts.DelaySeconds
	delayLbl := widget.NewLabel(fmt.Sprintf("Delay: %.1fs", delay.Value))
	delay.OnChanged = func(v float64) { delayLbl.SetText(fmt.Sprintf("Delay: %.1fs", v)) }

	fs := widget.NewSlider(10, 80)
	fs.Step = 1
	fs.Value = float64(opts.FontSize)
	fsLbl := widget.NewLabel(fmt.Sprintf("Font size: %d", opts.FontSize))
	fs.OnChanged = func(v float64) { fsLbl.SetText(fmt.Sprintf("Font size: %d", int(v))) }

	bold := widget.NewCheck("Bold", nil)
	bold.Checked = opts.Bold
	italic := widget.NewCheck("Italic", nil)
	italic.Checked = opts.Italic
	burn := widget.NewCheck("Burn-in", nil)
	burn.Checked = opts.BurnIn
	ignoreCC := widget.NewCheck("Ignore CC", nil)
	ignoreCC.Checked = opts.IgnoreClosedCaptions

	path := widget.NewEntry()
	path.SetText(opts.Path)

	form := container.NewVBox(
		delayLbl, delay,
		fsLbl, fs,
		bold, italic, burn, ignoreCC,
		widget.NewLabel("Path"), path,
	)
	dialog.ShowCustomConfirm("Subtitle", "Apply", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		next := options.SubtitleCastOptions{
			Path:                 path.Text,
			BurnIn:               burn.Checked,
			FontSize:             int(fs.Value),
			IgnoreClosedCaptions: ignoreCC.Checked,
			DelaySeconds:         delay.Value,
			Bold:                 bold.Checked,
			Italic:               italic.Checked,
		}
		u.currentSubtitleOpts = next
		u.currentSubtitlePath = next.Path
		go func() {
			if err := u.client.UpdateSubtitle(next); err != nil {
				fyne.Do(func() { u.fail(err) })
			}
		}()
	}, u.window)
}

// qualityBitrates are the transcode bitrates offered besides "Original".
var qualityBitrates = []string{"8M", "5M", "3M", "2M"}

// showTracksDialog switches the running cast's video track, audio track,
// quality or subtitle source. The TV reloads at the same position.
func (u *ui) showTracksDialog() {
	go func() {
		st, err := u.client.State()
		if err != nil {
			fyne.Do(func() { u.fail(err) })
			return
		}
		if st.Selection == nil || st.MediaPath == "" {
			fyne.Do(func() {
				dialog.ShowInformation("Tracks", "This instance doesn't report what it's playing.", u.window)
			})
			return
		}
		info, err := u.client.TrackInfo(st.MediaPath)
		if err != nil {
			fyne.Do(func() { u.fail(err) })
			return
		}
		sel := *st.Selection
		fyne.Do(func() { u.showTracksForm(sel, info) })
	}()
}

func (u *ui) showTracksForm(sel castapi.CastSelection, info *castapi.TrackDisplayInfo) {
	var videoLabels []string
	videoIdx := -1
	for i, v := range info.VideoTracks {
		videoLabels = append(videoLabels, fmt.Sprintf("%d: %s", v.Index, v.Resolution))
		if v.Index == sel.VideoTrack {
			videoIdx = i
		}
	}
	videoSel := widget.NewSelect(videoLabels, nil)
	if videoIdx >= 0 {
		videoSel.SetSelectedIndex(videoIdx)
	}

	var audioLabels []string
	audioIdx := -1
	for i, a := range info.AudioTracks {
		audioLabels = append(audioLabels, fmt.Sprintf("%d: %s", a.Index, a.Language))
		if a.Index == sel.AudioTrack {
			audioIdx = i
		}
	}
	audioSel := widget.NewSelect(audioLabels, nil)
	if audioIdx >= 0 {
		audioSel.SetSelectedIndex(audioIdx)
	}

	qualities := append([]string{""}, qualityBitrates...)
	qualityLabels := append([]string{"Original"}, qualityBitrates...)
	qualitySel := widget.NewSelect(qualityLabels, nil)
	for i, q := range qualities {
		if q == sel.Quality {
			qualitySel.SetSelectedIndex(i)
		}
	}

	// Subtitle sources on the casting instance: none, embedded tracks, and
	// the external files it knows about.
	var subPaths, subLabels []string
	for _, s := range info.SubtitleTracks {
		if s.Path != "external" {
			subPaths = append(subPaths, s.Path)
			subLabels = append(subLabels, s.Label)
		}
	}
	for _, p := range []string{sel.SubtitlePath, "external:" + info.NearSubtitle} {
		if !strings.HasPrefix(p, "external:") || p == "external:" || slices.Contains(subPaths, p) {
			continue
		}
		subPaths = append(subPaths, p)
		file := strings.TrimPrefix(p, "external:")
		subLabels = append(subLabels, "External: "+file[strings.LastIndexAny(file, `/\`)+1:])
	}
	subSel := widget.NewSelect(subLabels, nil)
	if i := slices.Index(subPaths, sel.SubtitlePath); i >= 0 {
		subSel.SetSelectedIndex(i)
	}

	form := container.NewVBox(
		widget.NewLabel("Video track"), videoSel,
		widget.NewLabel("Quality"), qualitySel,
	)
	if len(audioLabels) > 0 {
		form.Add(widget.NewLabel("Audio track"))
		form.Add(audioSel)
	}
	form.Add(widget.NewLabel("Subtitle"))
	form.Add(subSel)

	dialog.ShowCustomConfirm("Tracks", "Apply", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		var change castapi.TrackChange
		if i := videoSel.SelectedIndex(); i >= 0 && info.VideoTracks[i].Index != sel.VideoTrack {
			change.VideoTrack = &info.VideoTracks[i].Index
		}
		if i := audioSel.SelectedIndex(); i >= 0 && info.AudioTracks[i].Index != sel.AudioTrack {
			change.AudioTrack = &info.AudioTracks[i].Index
		}
		if i := qualitySel.SelectedIndex(); i >= 0 && qualities[i] != sel.Quality {
			change.Quality = &qualities[i]
		}
		if i := subSel.SelectedIndex(); i >= 0 && subPaths[i] != sel.SubtitlePath {
			change.SubtitlePath = &subPaths[i]
		}
		go func() {
			if _, err := u.client.ChangeTracks(change); err != nil {
				fyne.Do(func() { u.fail(err) })
			}
		}()
	}, u.window)
}

func (u *ui) seekRelative(current, delta float64) {
	target := current + delta
	if target < 0 {
		target = 0
	}
	go func() {
		if _, err := u.client.Control("seek", target); err != nil {
			fyne.Do(func() { u.fail(err) })
		}
	}()
}
