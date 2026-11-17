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

- **Connect**: searches the network on open (one-shot mDNS queries answered by
  unicast, so it works without Android's multicast lock), offers the last
  instance used, or an address and token.
- **Library**: shows, then seasons, then episodes, from the instance's library
  tree. An episode opens the play sheet: cast target (remembered), audio,
  subtitles (the file beside the episode is picked by default) and quality.
- **Web page** (from Library, or Share › Frog Cast Remote in a browser): cast
  a page link; recent pages are kept on the phone. The app closes when it
  leaves the screen, since Fyne's activity ignores a share sent to a running
  app. The instance extracts pages it has a script for. For any other page
  it answers 409, and the page opens full screen on the phone: tap play, and
  once it requests its stream the page closes and the stream, its headers,
  cookies and subtitles go to `/play-extracted`. Back closes the page and
  cancels. This needs Android 8 and a WebView with document-start scripts
  (any current one).
- **Playing**: the running cast, polled every second while on screen: seek,
  ±10/30 s, pause, volume and mute, stop. **Subtitles** adjusts timing, size
  and style live; **Tracks** switches video, quality, audio and subtitle
  source (the TV reloads at the same spot). Opens here when something is
  already playing.
- **Torrents**: add a magnet link; downloads with progress.
- **Manage**: rescan, identify shows on TMDB, organize files (with a preview),
  and translate a season's subtitles with progress and cancel.
