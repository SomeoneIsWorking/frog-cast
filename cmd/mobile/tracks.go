package main

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"frog-cast/pkg/castapi"
	"frog-cast/pkg/options"
)

// Transcode qualities: "" plays the original stream.
var (
	qualityValues = []string{"", "8M", "5M", "3M", "2M"}
	qualityLabels = []string{"Original", "Very high (8 Mbps)", "High (5 Mbps)", "Medium (3 Mbps)", "Low (2 Mbps)"}
)

// subtitleOptions are the subtitle sources on the casting instance.
type subtitleOptions struct {
	paths  []string
	labels []string
}

func (o subtitleOptions) index(path string) int {
	if i := slices.Index(o.paths, path); i >= 0 {
		return i
	}
	return 0
}

// subtitleChoices lists none, the embedded tracks and the external files the
// instance knows about: the current one and the one beside the media.
func subtitleChoices(info *castapi.TrackDisplayInfo, current string) subtitleOptions {
	var o subtitleOptions
	for _, s := range info.SubtitleTracks {
		if s.Path != "external" {
			o.paths = append(o.paths, s.Path)
			o.labels = append(o.labels, s.Label)
		}
	}
	for _, p := range []string{current, "external:" + info.NearSubtitle} {
		if !strings.HasPrefix(p, "external:") || p == "external:" || slices.Contains(o.paths, p) {
			continue
		}
		file := strings.TrimPrefix(p, "external:")
		o.paths = append(o.paths, p)
		o.labels = append(o.labels, "File: "+file[strings.LastIndexAny(file, `/\`)+1:])
	}
	return o
}

// showTracksSheet switches the running cast's video track, audio track,
// quality or subtitle source. The TV reloads at the same position.
func (s *session) showTracksSheet(st *castapi.PlaybackState) {
	if st.Selection == nil {
		s.ui.inform("Tracks", "This instance doesn't report what it's playing. Update it to switch tracks.")
		return
	}
	sel := *st.Selection
	go func() {
		info, err := s.client.TrackInfo(st.MediaPath)
		fyne.Do(func() {
			if err != nil {
				s.ui.fail(err)
				return
			}
			s.tracksSheet(sel, info)
		})
	}()
}

func (s *session) tracksSheet(sel castapi.CastSelection, info *castapi.TrackDisplayInfo) {
	var videoLabels []string
	for _, v := range info.VideoTracks {
		videoLabels = append(videoLabels, videoLabel(v))
	}
	video := widget.NewSelect(videoLabels, nil)
	for i, v := range info.VideoTracks {
		if v.Index == sel.VideoTrack {
			video.SetSelectedIndex(i)
		}
	}

	var audioLabels []string
	for _, a := range info.AudioTracks {
		audioLabels = append(audioLabels, trackLabel(a.Index, a.Language))
	}
	audio := widget.NewSelect(audioLabels, nil)
	for i, a := range info.AudioTracks {
		if a.Index == sel.AudioTrack {
			audio.SetSelectedIndex(i)
		}
	}

	quality := widget.NewSelect(qualityLabels, nil)
	if i := slices.Index(qualityValues, sel.Quality); i >= 0 {
		quality.SetSelectedIndex(i)
	}

	subs := subtitleChoices(info, sel.SubtitlePath)
	subtitle := widget.NewSelect(subs.labels, nil)
	subtitle.SetSelectedIndex(subs.index(sel.SubtitlePath))

	items := []*widget.FormItem{}
	if len(videoLabels) > 1 {
		items = append(items, widget.NewFormItem("Video", video))
	}
	items = append(items, widget.NewFormItem("Quality", quality))
	if len(audioLabels) > 1 {
		items = append(items, widget.NewFormItem("Audio", audio))
	}
	items = append(items, widget.NewFormItem("Subtitles", subtitle))

	switchTracks := func() {
		var change castapi.TrackChange
		if i := video.SelectedIndex(); i >= 0 && info.VideoTracks[i].Index != sel.VideoTrack {
			change.VideoTrack = &info.VideoTracks[i].Index
		}
		if i := audio.SelectedIndex(); i >= 0 && info.AudioTracks[i].Index != sel.AudioTrack {
			change.AudioTrack = &info.AudioTracks[i].Index
		}
		if i := quality.SelectedIndex(); i >= 0 && qualityValues[i] != sel.Quality {
			change.Quality = &qualityValues[i]
		}
		if i := subtitle.SelectedIndex(); i >= 0 && subs.paths[i] != sel.SubtitlePath {
			change.SubtitlePath = &subs.paths[i]
		}
		go func() {
			if _, err := s.client.ChangeTracks(change); err != nil {
				fyne.Do(func() { s.ui.fail(err) })
			}
		}()
	}
	s.ui.showSheet("Tracks", sheetFields(items...),
		sheetAction{label: "Cancel", importance: widget.LowImportance},
		sheetAction{label: "Switch", importance: widget.HighImportance, tapped: switchTracks},
	)
}

// showSubtitleSheet adjusts the running cast's subtitle size, timing and
// style; each change applies at once. The casting instance keeps the timing
// for this episode.
func (s *session) showSubtitleSheet(st *castapi.PlaybackState) {
	opts := options.SubtitleCastOptions{FontSize: 24}
	if sel := st.Selection; sel != nil {
		opts = options.SubtitleCastOptions{
			BurnIn:               sel.BurnIn,
			FontSize:             sel.FontSize,
			IgnoreClosedCaptions: sel.IgnoreClosedCaptions,
			DelaySeconds:         sel.DelaySeconds,
			Bold:                 sel.Bold,
			Italic:               sel.Italic,
		}
	}
	// An empty Path keeps the subtitle source; switching it is in Tracks.
	opts.Path = ""
	apply := func() {
		next := opts
		go func() {
			if err := s.client.UpdateSubtitle(next); err != nil {
				fyne.Do(func() { s.ui.fail(err) })
			}
		}()
	}

	delayLabel := widget.NewLabel("")
	showDelay := func() { delayLabel.SetText(fmt.Sprintf("%+.1f s", opts.DelaySeconds)) }
	showDelay()
	nudge := func(by float64) *widget.Button {
		return widget.NewButton(fmt.Sprintf("%+g", by), func() {
			opts.DelaySeconds = math.Round((opts.DelaySeconds+by)*10) / 10
			showDelay()
			apply()
		})
	}
	reset := widget.NewButton("0", func() { opts.DelaySeconds = 0; showDelay(); apply() })
	timing := container.NewVBox(
		container.NewCenter(delayLabel),
		container.NewGridWithColumns(5, nudge(-0.5), nudge(-0.1), reset, nudge(0.1), nudge(0.5)),
		caption("Positive shows subtitles later."),
	)

	sizeLabel := widget.NewLabel(fmt.Sprint(opts.FontSize))
	size := widget.NewSlider(10, 120)
	size.Step = 2
	size.Value = float64(opts.FontSize)
	size.OnChanged = func(v float64) { sizeLabel.SetText(fmt.Sprint(int(v))) }
	size.OnChangeEnded = func(v float64) { opts.FontSize = int(v); apply() }

	bold := widget.NewCheck("Bold", func(on bool) { opts.Bold = on; apply() })
	bold.Checked = opts.Bold
	italic := widget.NewCheck("Italic", func(on bool) { opts.Italic = on; apply() })
	italic.Checked = opts.Italic

	content := sheetFields(
		widget.NewFormItem("Timing", timing),
		widget.NewFormItem("Size", container.NewBorder(nil, nil, nil, sizeLabel, size)),
		widget.NewFormItem("Style", container.NewHBox(bold, italic)),
	)
	s.ui.showSheet("Subtitles", content, sheetAction{label: "Done", importance: widget.HighImportance})
}
