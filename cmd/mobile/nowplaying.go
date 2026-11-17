package main

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"wails-cast/pkg/castapi"
)

// nowPlaying shows and controls the instance's running cast. It polls the
// instance's state once a second while it is on screen.
type nowPlaying struct {
	s       *session
	content fyne.CanvasObject

	state    *castapi.PlaybackState
	stopPoll chan struct{}
	seeking  bool // the user is dragging the position slider

	idle      fyne.CanvasObject
	active    fyne.CanvasObject
	where     *widget.Label
	title     *widget.Label
	elapsed   *widget.Label
	remaining *widget.Label
	position  *widget.Slider
	playPause *widget.Button
	mute      *widget.Button
	volume    *widget.Slider
}

func newNowPlaying(s *session) *nowPlaying {
	p := &nowPlaying{s: s}

	p.where = caption("")
	p.title = heading1("")
	p.title.Wrapping = fyne.TextWrapWord

	p.elapsed = widget.NewLabel("0:00")
	p.remaining = widget.NewLabel("0:00")
	p.position = widget.NewSlider(0, 1)
	p.position.OnChanged = func(v float64) {
		p.seeking = true
		p.elapsed.SetText(fmtSec(v))
	}
	p.position.OnChangeEnded = func(v float64) {
		p.seeking = false
		p.control("seek", v)
	}

	p.playPause = widget.NewButtonWithIcon("", theme.MediaPauseIcon(), p.togglePause)
	p.playPause.Importance = widget.HighImportance
	skip := func(icon fyne.Resource, by float64) *widget.Button {
		return widget.NewButtonWithIcon("", icon, func() { p.seekBy(by) })
	}
	transport := container.NewGridWithColumns(5,
		skip(theme.MediaSkipPreviousIcon(), -30),
		skip(theme.MediaFastRewindIcon(), -10),
		p.playPause,
		skip(theme.MediaFastForwardIcon(), 10),
		skip(theme.MediaSkipNextIcon(), 30),
	)
	jumps := container.NewGridWithColumns(5, note("−30s"), note("−10s"), widget.NewLabel(""), note("+10s"), note("+30s"))
	for _, o := range jumps.Objects {
		o.(*widget.Label).Alignment = fyne.TextAlignCenter
	}

	p.volume = widget.NewSlider(0, 1)
	p.volume.Step = 0.01
	p.volume.OnChangeEnded = func(v float64) { p.control("volume", v) }
	p.mute = widget.NewButtonWithIcon("", theme.VolumeUpIcon(), p.toggleMute)
	p.mute.Importance = widget.LowImportance

	subtitles := widget.NewButtonWithIcon("Subtitles", theme.DocumentIcon(), func() {
		if p.state != nil {
			s.showSubtitleSheet(p.state)
		}
	})
	tracks := widget.NewButtonWithIcon("Tracks", theme.ListIcon(), func() {
		if p.state != nil {
			s.showTracksSheet(p.state)
		}
	})
	stop := widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), func() { p.control("stop", 0) })
	stop.Importance = widget.DangerImportance

	p.active = container.NewVBox(
		p.where,
		p.title,
		widget.NewSeparator(),
		p.position,
		container.NewBorder(nil, nil, p.elapsed, p.remaining),
		transport,
		jumps,
		widget.NewSeparator(),
		container.NewBorder(nil, nil, p.mute, nil, p.volume),
		container.NewGridWithColumns(2, subtitles, tracks),
		layoutSpacer(),
		stop,
	)
	p.idle = container.NewCenter(container.NewVBox(
		widget.NewIcon(theme.MediaVideoIcon()),
		widget.NewLabelWithStyle("Nothing is playing", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		note("Pick an episode in Library."),
	))
	p.active.Hide()
	p.content = container.NewPadded(container.NewStack(p.idle, container.NewVScroll(p.active)))
	return p
}

func layoutSpacer() fyne.CanvasObject {
	return container.NewPadded(widget.NewLabel(""))
}

// setPolling starts or stops following the instance's state.
func (p *nowPlaying) setPolling(on bool) {
	if !on {
		if p.stopPoll != nil {
			close(p.stopPoll)
			p.stopPoll = nil
		}
		return
	}
	if p.stopPoll != nil {
		return
	}
	stop := make(chan struct{})
	p.stopPoll = stop
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if st, err := p.s.client.State(); err == nil {
				fyne.Do(func() { p.show(st) })
			}
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
}

func (p *nowPlaying) show(st *castapi.PlaybackState) {
	p.state = st
	if st.Status == "STOPPED" {
		p.active.Hide()
		p.idle.Show()
		return
	}
	p.idle.Hide()
	p.active.Show()

	status := map[string]string{"PLAYING": "Playing", "PAUSED": "Paused", "LOADING": "Loading"}[st.Status]
	if status == "" {
		status = st.Status
	}
	p.where.SetText(status + " on " + p.s.targets.name(st.DeviceURL))
	if p.title.Text != st.MediaName {
		p.title.SetText(st.MediaName)
		// The title's wrapped height changed; lay the screen out again.
		p.active.Refresh()
	}

	if !p.seeking {
		p.position.Max = max(st.Duration, 1)
		p.position.Value = min(st.CurrentTime, p.position.Max)
		p.position.Refresh()
		p.elapsed.SetText(fmtSec(st.CurrentTime))
	}
	p.remaining.SetText("−" + fmtSec(max(st.Duration-st.CurrentTime, 0)))

	if st.Status == "PAUSED" {
		p.playPause.SetIcon(theme.MediaPlayIcon())
	} else {
		p.playPause.SetIcon(theme.MediaPauseIcon())
	}
	if st.Muted {
		p.mute.SetIcon(theme.VolumeMuteIcon())
	} else {
		p.mute.SetIcon(theme.VolumeUpIcon())
	}
	p.volume.Value = st.Volume
	p.volume.Refresh()
}

func (p *nowPlaying) togglePause() {
	if p.state != nil && p.state.Status == "PAUSED" {
		p.control("resume", 0)
	} else {
		p.control("pause", 0)
	}
}

func (p *nowPlaying) toggleMute() {
	if p.state != nil && p.state.Muted {
		p.control("unmute", 0)
	} else {
		p.control("mute", 0)
	}
}

func (p *nowPlaying) seekBy(seconds float64) {
	if p.state == nil {
		return
	}
	target := min(max(p.state.CurrentTime+seconds, 0), p.state.Duration)
	p.control("seek", target)
}

// control sends a transport command and shows the state it returns.
func (p *nowPlaying) control(action string, value float64) {
	go func() {
		st, err := p.s.client.Control(action, value)
		fyne.Do(func() {
			if err != nil {
				p.s.ui.fail(err)
				return
			}
			p.show(st)
		})
	}()
}
