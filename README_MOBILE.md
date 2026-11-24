# Frog Cast Remote

A Fyne v2 remote for a frog-cast instance on the LAN (`cmd/mobile`). It uses
the shared `pkg/castapi` client (mDNS discovery and the HTTP API) that the
desktop app also uses.

## Build & run

Desktop preview:

```
go run ./cmd/mobile
```

Render every screen to PNGs against a running instance, without a window
(read-only calls only):

```
go test ./cmd/mobile -run RenderScreens -args -shots <dir> -base http://<host>:9999
```

Package for Android from `cmd/mobile/` (the packager won't take another
directory for mobile targets):

```
go install fyne.io/tools/cmd/fyne@v1.7.4
export ANDROID_HOME=<sdk> ANDROID_NDK_HOME=<sdk>/ndk/<version>
go generate ./pkg/webextract   # the WebView bridge (needs a JDK)
cd cmd/mobile
fyne package -os android/arm64 --app-id io.frogcast.remote --name "Frog Cast Remote" --icon ../../build/appicon.png
adb install -r Frog_Cast_Remote.apk
```

`-os android` builds every ABI (about 130 MB); `android/arm64` is enough for
current phones. The APK is debug-signed; pass `--release` and sign it with
`apksigner` for a release.

iOS (macOS + Xcode): `fyne package -os ios --app-id io.frogcast.remote --icon ../../build/appicon.png`.

The launcher icon is the desktop app's (`build/appicon.png`).

## Screens

Choices and confirmations open as sheets from the bottom of the screen; a tap
outside the sheet or Back closes it.

- **Connect**: searches the network on open (one-shot mDNS queries answered by
  unicast, so it works without Android's multicast lock), offers the last
  instance used, or an address and token.
- **Library**: shows, then seasons, then episodes, from the instance's library
  tree. An episode opens the play sheet: cast target (remembered), audio,
  subtitles (the file beside the episode is picked by default) and quality.
- **History** (from Library): what the instance has cast, newest first.
  A file opens its play sheet, a page its Cast options; remove one or clear
  all.
- **Web page** (from Library, or Share › Frog Cast Remote in a browser): cast
  a page link. Opening a page shows its
  **Cast options**: target, video (the tallest variant up to 1080p is picked
  first, since every variant is re-encoded at its own size and many TVs
  refuse more), audio, subtitles and quality, with a download bar and button
  per track to pre-download it to the instance. The app closes when it
  leaves the screen, since Fyne's activity ignores a share sent to a running
  app. The instance extracts pages it has a script for. For any other page
  it answers 409, and the page opens full screen on the phone: tap play, and
  once it requests its stream the page closes and the stream, its headers,
  cookies and subtitles go to `/extracted`. Back closes the page and
  cancels. This needs Android 8 and a WebView with document-start scripts
  (any current one).
- **Playing**: the running cast, polled every second while on screen: seek,
  ±10/30 s, pause, volume (slider, and − / + one notch of the TV's 0–100) and
  mute, stop. It names the video, quality, audio and subtitles playing and,
  for a web page, shows its video and audio downloads into the instance.
  **Subtitles** adjusts timing, size and style live; **Tracks** switches
  video, quality, audio and subtitle source (the TV reloads at the same
  spot); **Recast** loads the cast on the TV again where it was, for a TV
  that dropped the session asleep after a long pause (also offered once the
  cast reads as stopped). Opens here when something is already playing.
  Render it against an instance with
  `go test ./cmd/mobile -run RenderPlaying -args -shots DIR -base URL -page PAGE`.
- **Downloads**: the instance's torrents (add a magnet link) and its web page
  track downloads, with progress; a page track can be stopped or resumed.
- **Manage**: the instance's media cache size, clearing transcoded
  segments, all video or everything (each confirmed); rescan, identify shows on TMDB, organize files (with a preview),
  and translate a season's subtitles with progress and cancel.
