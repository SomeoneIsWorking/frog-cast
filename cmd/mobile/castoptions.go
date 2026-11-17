package main

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
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
	o.video = newTrackDownload(o, "video")
	video.OnChanged = func(string) { o.video.setTrack(info.VideoTracks[video.SelectedIndex()].Index) }
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
	o.audio = newTrackDownload(o, "audio")
	audio.OnChanged = func(string) { o.audio.setTrack(info.AudioTracks[audio.SelectedIndex()].Index) }
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

// trackDownload shows and drives the download of the chosen track of one
// kind into the instance's cache.
type trackDownload struct {
	o        *pageOptions
	kind     string
	track    int
	content  fyne.CanvasObject
	button   *widget.Button
	progress *thinBar
	label    *widget.Label
	status   castapi.DownloadStatus
	polling  bool
}

func newTrackDownload(o *pageOptions, kind string) *trackDownload {
	d := &trackDownload{o: o, kind: kind, track: -1}
	d.button = widget.NewButtonWithIcon("", theme.DownloadIcon(), d.toggle)
	d.progress = newThinBar()
	d.label = note("")
	d.label.SizeName = theme.SizeNameCaptionText
	d.label.Truncation = fyne.TextTruncateEllipsis
	bar := container.NewVBox(layout.NewSpacer(), d.progress, layout.NewSpacer())
	d.content = container.NewBorder(nil, nil, nil, d.button, container.New(layout.NewCustomPaddedVBoxLayout(-6), bar, d.label))
	return d
}

func (d *trackDownload) setTrack(track int) {
	d.track = track
	d.status = castapi.DownloadStatus{}
	d.show()
	d.refresh()
}

func (d *trackDownload) refresh() {
	client, pageURL, kind, track := d.o.v.s.client, d.o.pageURL, d.kind, d.track
	go func() {
		st, err := client.Download(pageURL, kind, track)
		fyne.Do(func() {
			if err != nil || track != d.track {
				return
			}
			d.status = *st
			d.show()
			d.follow()
		})
	}()
}

func (d *trackDownload) toggle() {
	client, req := d.o.v.s.client, castapi.DownloadRequest{
		URL: d.o.pageURL, Type: d.kind, Track: d.track, Start: d.status.Status != "INPROGRESS",
	}
	d.button.Disable()
	go func() {
		st, err := client.SetDownload(req)
		fyne.Do(func() {
			d.button.Enable()
			if err != nil {
				d.o.v.s.ui.fail(err)
				return
			}
			if req.Track == d.track {
				d.status = *st
				d.show()
				d.follow()
			}
		})
	}()
}

// follow polls the download while it runs and the page is on screen.
func (d *trackDownload) follow() {
	if d.polling || d.status.Status != "INPROGRESS" {
		return
	}
	d.polling = true
	go func() {
		time.Sleep(2 * time.Second)
		fyne.Do(func() {
			d.polling = false
			if d.o.visible() {
				d.refresh()
			}
		})
	}()
}

func (d *trackDownload) show() {
	st := d.status
	if st.Total > 0 {
		d.progress.SetValue(float64(st.Done) / float64(st.Total))
	} else {
		d.progress.SetValue(0)
	}
	switch st.Status {
	case "INPROGRESS":
		d.button.SetIcon(theme.MediaStopIcon())
		d.label.SetText(fmt.Sprintf("Downloading · %d of %d segments", st.Done, st.Total))
	case "COMPLETED":
		d.button.SetIcon(theme.ConfirmIcon())
		d.label.SetText(fmt.Sprintf("Downloaded · %d segments", st.Total))
	case "ERROR":
		d.button.SetIcon(theme.DownloadIcon())
		d.label.SetText(fmt.Sprintf("Download failed · %d of %d segments", st.Done, st.Total))
	case "":
		d.button.SetIcon(theme.DownloadIcon())
		d.label.SetText("Checking the cache…")
	default:
		d.button.SetIcon(theme.DownloadIcon())
		d.label.SetText(fmt.Sprintf("%d of %d segments downloaded", st.Done, st.Total))
	}
	d.button.Disable()
	if st.Status != "" && st.Status != "COMPLETED" {
		d.button.Enable()
	}
}
