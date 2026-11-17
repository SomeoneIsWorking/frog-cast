package castapi

// CastInstance is a wails-cast instance discovered on the LAN over mDNS.
type CastInstance struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
	URL  string `json:"url"`
}

// RemoteDevice is a cast target reported by a remote instance's /devices.
type RemoteDevice struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
	UUID string `json:"uuid"`
}

// PlayOptions is the optional track/subtitle/quality selection accepted by
// the remote /play and /play-url endpoints.
type PlayOptions struct {
	VideoTrack   int     `json:"videoTrack"`
	AudioTrack   int     `json:"audioTrack"`
	SubtitlePath string  `json:"subtitlePath"` // "" or "none" = no subtitle
	Quality      *string `json:"quality"`      // nil = the instance's default; "" = Original
	// Subtitle is the starting controller's subtitle style; nil uses the
	// casting instance's settings.
	Subtitle *SubtitleStyle `json:"subtitle,omitempty"`
}

// SubtitleStyle is a controller's default subtitle look for casts it starts.
type SubtitleStyle struct {
	FontSize int  `json:"fontSize"`
	Bold     bool `json:"bold"`
	Italic   bool `json:"italic"`
	BurnIn   bool `json:"burnIn"`
}

type LibraryItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Duration int    `json:"duration,omitempty"`
}

type PlaybackState struct {
	Status      string  `json:"status"`
	MediaPath   string  `json:"mediaPath"`
	MediaName   string  `json:"mediaName"`
	DeviceURL   string  `json:"deviceUrl"`
	DeviceName  string  `json:"deviceName"`
	CurrentTime float64 `json:"currentTime"`
	Duration    float64 `json:"duration"`
	Volume      float64 `json:"volume"`
	Muted       bool    `json:"muted"`
	// Selection is what the current cast plays. Older instances omit it, and
	// a client treats a missing selection as unknown.
	Selection *CastSelection `json:"selection,omitempty"`
}

// TrackChange is the body of POST /tracks: the parts of a running cast's
// selection to switch. A nil field keeps the current choice.
type TrackChange struct {
	VideoTrack   *int    `json:"videoTrack,omitempty"`
	AudioTrack   *int    `json:"audioTrack,omitempty"`
	Quality      *string `json:"quality,omitempty"`
	SubtitlePath *string `json:"subtitlePath,omitempty"`
}

// CastSelection is the track, quality and subtitle choice of a running cast.
type CastSelection struct {
	VideoTrack   int     `json:"videoTrack"`
	AudioTrack   int     `json:"audioTrack"`
	Quality      string  `json:"quality"`
	SubtitlePath string  `json:"subtitlePath"`
	FontSize     int     `json:"fontSize"`
	Bold         bool    `json:"bold"`
	Italic       bool    `json:"italic"`
	DelaySeconds float64 `json:"delaySeconds"`
	BurnIn       bool    `json:"burnIn"`
	// IgnoreClosedCaptions drops closed-caption cues from the subtitles.
	IgnoreClosedCaptions bool `json:"ignoreClosedCaptions"`
}

type SubtitleDisplayItem struct {
	Path  string
	Label string
}

type AudioTracksDisplayItem struct {
	Index    int
	Language string
}

type VideoTrackDisplayItem struct {
	Index      int
	Codecs     string
	Resolution string
}

type TrackDisplayInfo struct {
	VideoTracks    []VideoTrackDisplayItem
	AudioTracks    []AudioTracksDisplayItem
	SubtitleTracks []SubtitleDisplayItem
	Path           string
	NearSubtitle   string
}

type QualityOption struct {
	Label   string
	Key     string
	Default bool
}

type TorrentStatus struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	Progress    float64 `json:"progress"`
	State       string  `json:"state"`
	DlSpeed     int64   `json:"dlspeed"`
	Eta         int64   `json:"eta"`
	Size        int64   `json:"size"`
	ContentPath string  `json:"content_path"`
	SavePath    string  `json:"save_path"`
}

type LibraryEpisode struct {
	Path         string `json:"path"`
	Name         string `json:"name"`
	Season       int    `json:"season"`
	Episode      int    `json:"episode"`
	HasSubtitles bool   `json:"hasSubtitles"`
	Translated   bool   `json:"translated"`
	EpisodeName  string `json:"episodeName"`
	Identified   bool   `json:"identified"`
}

type LibrarySeason struct {
	Name     string           `json:"name"`
	Number   int              `json:"number"`
	Episodes []LibraryEpisode `json:"episodes"`
}

type LibraryShow struct {
	Name       string          `json:"name"`
	Path       string          `json:"path"`
	Seasons    []LibrarySeason `json:"seasons"`
	TMDBID     int             `json:"tmdbId"`
	IMDBID     string          `json:"imdbId"`
	Year       int             `json:"year"`
	Identified bool            `json:"identified"`
}

type LibraryScanResult struct {
	RootPath string        `json:"rootPath"`
	Shows    []LibraryShow `json:"shows"`
}

type OrganizeMove struct {
	SrcVideo    string `json:"srcVideo"`
	DstVideo    string `json:"dstVideo"`
	SrcSubDir   string `json:"srcSubDir"`
	DstSubDir   string `json:"dstSubDir"`
	Description string `json:"description"`
}

type LibraryIdentifyProgress struct {
	Total    int    `json:"total"`
	Current  int    `json:"current"`
	ShowName string `json:"showName"`
	Status   string `json:"status"`
	Message  string `json:"message"`
}

type SeasonTranslateProgress struct {
	ShowName       string `json:"showName"`
	SeasonName     string `json:"seasonName"`
	TargetLanguage string `json:"targetLanguage"`
	TotalEpisodes  int    `json:"totalEpisodes"`
	CurrentEpisode int    `json:"currentEpisode"`
	Status         string `json:"status"`
	Message        string `json:"message"`
}

type TranslateStatus struct {
	InProgress bool     `json:"inProgress"`
	Language   string   `json:"language"`
	Files      []string `json:"files"`
	Error      string   `json:"error"`
}

type PingResponse struct {
	OK        bool   `json:"ok"`
	AppName   string `json:"app"`
	Timestamp string `json:"timestamp"`
}

type LibraryResponse struct {
	Items []LibraryItem `json:"items"`
}

type PlayResponse struct {
	OK    bool          `json:"ok"`
	State PlaybackState `json:"state"`
}

type ErrorResponse struct {
	Error string `json:"error"`
	// NeedsExtraction: the page has to be extracted in a browser someone can
	// see first; send it with /play-extracted from a controller that can.
	NeedsExtraction bool `json:"needsExtraction,omitempty"`
}

// ExtractedPage is a web page's stream as a controller's browser found it,
// sent with POST /play-extracted.
type ExtractedPage struct {
	PageURL     string            `json:"pageUrl"`
	Title       string            `json:"title"`
	ManifestURL string            `json:"manifestUrl"` // absolute
	Manifest    string            `json:"manifest"`
	Headers     map[string]string `json:"headers"`
	Cookies     map[string]string `json:"cookies"`
}

// PlayExtractedRequest is the body of POST /play-extracted.
type PlayExtractedRequest struct {
	Page     ExtractedPage `json:"page"`
	DeviceIP string        `json:"deviceIp"` // "local" = the instance itself
	PlayOptions
}
