package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"frog-cast/pkg/castapi"
)

// pageOptions is a web page's Cast options: where to cast it, its video and
// audio tracks, each with a download into the instance's cache, its
// subtitles and quality.
type pageOptions struct {
	v       *webView
	pageURL string
	info    *castapi.TrackDisplayInfo
	content fyne.CanvasObject
	target  *widget.Select
	video   *trackDownload
	audio   *trackDownload
	closed  bool
}

func newPageOptions(v *webView, pageURL string, info *castapi.TrackDisplayInfo) *pageOptions {
	o := &pageOptions{v: v, pageURL: pageURL, info: info}
	s := v.s
	o.target = s.targets.newSelect()

	var videoLabels []string
	for _, t := range info.VideoTracks {
		videoLabels = append(videoLabels, videoLabel(t))
	}
	video := widget.NewSelect(videoLabels, nil)
	o.video = newTrackDownload(s, "video", o.visible)
	video.OnChanged = func(string) { o.video.setTrack(pageURL, info.VideoTracks[video.SelectedIndex()].Index) }
	if len(info.VideoTracks) > 0 {
		pick := variantFor(info.VideoTracks, defaultMaxHeight)
		for i, t := range info.VideoTracks {
			if t.Index == pick {
				video.SetSelectedIndex(i)
			}
		}
	}

	var audioLabels []string
	for _, a := range info.AudioTracks {
		audioLabels = append(audioLabels, trackLabel(a.Index, a.Language))
	}
	audio := widget.NewSelect(audioLabels, nil)
	o.audio = newTrackDownload(s, "audio", o.visible)
	audio.OnChanged = func(string) { o.audio.setTrack(pageURL, info.AudioTracks[audio.SelectedIndex()].Index) }
	if len(info.AudioTracks) > 0 {
		audio.SetSelectedIndex(0)
	}

	subs := subtitleChoices(info, "")
	subtitle := widget.NewSelect(subs.labels, nil)
	subtitle.SetSelectedIndex(0)

	quality := widget.NewSelect(qualityLabels, nil)
	quality.PlaceHolder = "Instance default"

	cast := widget.NewButtonWithIcon("Cast", theme.MediaPlayIcon(), nil)
	cast.Importance = widget.HighImportance
	cast.OnTapped = func() {
		host, err := s.targets.selected()
		if err != nil {
			s.ui.fail(err)
			return
		}
		opts := castapi.PlayOptions{SubtitlePath: "none"}
		if i := video.SelectedIndex(); i >= 0 {
			opts.VideoTrack = info.VideoTracks[i].Index
		}
		if i := audio.SelectedIndex(); i >= 0 {
			opts.AudioTrack = info.AudioTracks[i].Index
		}
		if i := subtitle.SelectedIndex(); i >= 0 {
			opts.SubtitlePath = subs.paths[i]
		}
		if i := quality.SelectedIndex(); i >= 0 {
			opts.Quality = &qualityValues[i]
		}
		cast.Disable()
		go func() {
			_, err := s.client.Play(pageURL, host, opts)
			fyne.Do(func() {
				cast.Enable()
				if err != nil {
					s.ui.fail(err)
					return
				}
				s.showPlaying()
			})
		}()
	}

	form := widget.NewForm(widget.NewFormItem("Cast to", o.target))
	if len(videoLabels) > 0 {
		form.Append("Video", container.NewVBox(video, o.video.content))
	}
	if len(audioLabels) > 0 {
		form.Append("Audio", container.NewVBox(audio, o.audio.content))
	}
	form.Append("Subtitles", subtitle)
	form.Append("Quality", quality)

	hint := caption("Download a track to play it from " + s.name + "'s disk, without waiting on the site.")
	o.content = container.NewVScroll(container.NewVBox(card(form), hint, cast))
	return o
}

// close stops following its downloads and lets go of its target picker.
func (o *pageOptions) close() {
	o.closed = true
	o.v.s.targets.release(o.target)
}

// visible reports whether the page is on screen; polls stop when it isn't.
func (o *pageOptions) visible() bool {
	return !o.closed && o.v.s.library.pages.top() == o.content && o.v.s.tabs.Selected() == tabLibrary
}
