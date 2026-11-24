package main

import (
	"fmt"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"frog-cast/pkg/castapi"
)

// playingTracks shows what the running cast plays (video and quality, audio,
// subtitles) and, for a web page, the download of its video and audio into
// the instance's cache.
type playingTracks struct {
	s       *session
	content fyne.CanvasObject

	video     *widget.Label
	audio     *widget.Label
	subtitles *widget.Label
	videoDL   *trackDownload
	audioDL   *trackDownload

	media string // the MediaPath info belongs to
	info  *castapi.TrackDisplayInfo
	sel   castapi.CastSelection
}

func newPlayingTracks(s *session, onScreen func() bool) *playingTracks {
	t := &playingTracks{s: s}
	value := func() *widget.Label {
		l := widget.NewLabel("")
		l.Truncation = fyne.TextTruncateEllipsis
		return l
	}
	t.video, t.audio, t.subtitles = value(), value(), value()
	t.videoDL = newTrackDownload(s, "video", onScreen)
	t.audioDL = newTrackDownload(s, "audio", onScreen)
	t.content = card(widget.NewForm(
		widget.NewFormItem("Video", container.NewVBox(t.video, t.videoDL.content)),
		widget.NewFormItem("Audio", container.NewVBox(t.audio, t.audioDL.content)),
		widget.NewFormItem("Subtitles", t.subtitles),
	))
	return t
}

// show follows the cast's selection; the media's track names are fetched once.
func (t *playingTracks) show(st *castapi.PlaybackState) {
	if st.Selection == nil {
		t.content.Hide()
		return
	}
	t.content.Show()
	t.sel = *st.Selection
	if st.MediaPath != t.media {
		t.media, t.info = st.MediaPath, nil
		media := st.MediaPath
		go func() {
			info, err := t.s.client.TrackInfo(media)
			fyne.Do(func() {
				if err != nil || media != t.media {
					return
				}
				t.info = info
				t.render()
			})
		}()
	}
	t.render()
}

func (t *playingTracks) render() {
	quality := "Original"
	if i := slices.Index(qualityValues, t.sel.Quality); i >= 0 {
		quality = qualityLabels[i]
	}
	video := fmt.Sprintf("Track %d", t.sel.VideoTrack)
	audio := fmt.Sprintf("Track %d", t.sel.AudioTrack)
	subtitles := "None"
	if t.info != nil {
		for _, v := range t.info.VideoTracks {
			if v.Index == t.sel.VideoTrack {
				video = videoLabel(v)
			}
		}
		for _, a := range t.info.AudioTracks {
			if a.Index == t.sel.AudioTrack && a.Language != "" {
				audio = a.Language
			}
		}
		subs := subtitleChoices(t.info, t.sel.SubtitlePath)
		subtitles = subs.labels[subs.index(t.sel.SubtitlePath)]
	}
	t.video.SetText(video + " · " + quality)
	t.audio.SetText(audio)
	t.subtitles.SetText(subtitles)

	if !isWebPage(t.media) {
		t.videoDL.content.Hide()
		t.audioDL.content.Hide()
		return
	}
	t.videoDL.content.Show()
	t.videoDL.setTrack(t.media, t.sel.VideoTrack)
	if t.info != nil && len(t.info.AudioTracks) > 0 {
		t.audioDL.content.Show()
		t.audioDL.setTrack(t.media, t.sel.AudioTrack)
	} else {
		t.audioDL.content.Hide()
	}
}

// resumeFollowing re-reads the downloads when the screen comes back, since
// they stop polling while it is away.
func (t *playingTracks) resumeFollowing() {
	for _, d := range []*trackDownload{t.videoDL, t.audioDL} {
		if d.pageURL != "" && d.track >= 0 {
			d.refresh()
		}
	}
}
