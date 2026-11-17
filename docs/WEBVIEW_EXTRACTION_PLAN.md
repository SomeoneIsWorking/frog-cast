# WebView extraction (no devtools, no Chrome)

Status (2026-11-17): the app extracts through `pkg/webextract`: WebKitGTK on
Linux, with no Chrome or devtools; Chromium, running the same script, on
other platforms until a native host exists. The phone and macOS hosts are
open (see Next).

## Why

`pkg/extractor` drives a downloaded Chromium over the devtools protocol. That
is a large dependency, it can't run on a phone, and the sites ship scripts
that detect devtools (every Lua script blocks `disable-devtool`). A page
loaded in a plain WebView with a script injected into every frame needs
neither: the script sees the requests the page makes and reports them to the
host.

## How it works

`capture.js` runs at document start in every frame, cross-origin player
iframes included. It wraps `fetch` and `XMLHttpRequest` and reads small text
responses, reporting:

- an HLS playlist by content (`#EXTM3U`), whatever the URL looks like
  (`master.txt`, `.png`), marking master playlists;
- WebVTT subtitles with their content, leaving out seek-preview thumbnail
  tracks (`#xywh=`);
- a playlist URL set directly on a media element (native HLS);
- per-site rules: selectors to click once the frame has loaded (play
  overlays), and bodies or frame HTML for a script's intercept handler to
  find the stream URL in.

It also switches lazy iframes to eager, since nobody scrolls the view. Messages
go to `window.webkit.messageHandlers.cast` (WebKit), `window.CastBridge` (an
Android WebView) or the `__castPost` devtools binding (Chromium).

The Linux host (`webkit_linux.go`) loads the page in an offscreen
`WebKitWebView` on the GTK main loop, with an ephemeral context, autoplay
allowed, sound muted, no GPU compositing, and a content blocker for the
rule's blocked URLs. It picks a master playlist, else a URL found in page
text, else a media playlist, and keeps listening briefly for subtitles. It
returns the playlist, the subtitles, the Referer (the requesting frame), the
Origin when cross-origin, the WebView's User-Agent and the cookies WebKit
holds for the playlist URL.

The Chromium host (`chromium.go`, built on non-Linux or with `-tags
chromium`) launches Chromium with site isolation off, so the script and
binding reach every frame, and claims codec support, which Chromium builds
lack and players check before requesting a stream.

Site rules come from the Lua scripts (`docs/LUA_SCRIPTS.md`):
`pkg/extractor` turns `block_urls`, `click` and `intercepts` into `Rules` and
runs the intercept handlers on the bodies the page reports. Without a script
the page opens in a window for someone to click play. Try a site with
`go run ./cmd/extract <page>`, which prints every message.

## Results (2026-11-17, from Fedora, WebKit host)

| Site | Rule | Found | Plays through `/play-extracted` |
| --- | --- | --- | --- |
| fullhdfilmizlesene.now | click `.ply img` | master in 5.7 s, plus the player's Turkish VTT (the Chrome path took 32 s and finds no subtitles there) | yes: frame at 25:00 and the subtitles |
| hdfilmcehennemi.nl | click `.play-that-video` | master (`master.txt`) in 6.1 s, plus 5 VTTs | yes: frame at 30:00 and the Turkish subtitles |
| fullhdfilmizle (now `.fit`) | click `#playerPoster` | master in 6.5 s, plus the Turkish VTT | no: fastplay.mom answers 404 to every request from outside the WebView, with the same token, IP and headers |
| diziwatch.ac | | not tried: TLS to the site fails from this network | |

None of the three needed the Lua scripts' workaround of navigating the top
page into the player iframe. The click rules differ from the Lua ones: a
synthetic click has to hit the element that owns the handler (the cover
image under fullhdfilmizlesene's `#play-video`, which has
`pointer-events: none`), where rod clicked by mouse position.

fastplay.mom's 404 hits the Chrome path too, since both hand downloads to Go's
HTTP client. Some client fingerprint gives non-browser clients away. The fix
would be fetching that site's playlists and segments through the WebView;
not attempted.

Through the app's `/play-url`, hdfilmcehennemi extracted in 6 s on the app's
own GTK loop and cast to a local player. The unscripted path opened the hls.js
demo page and found its master in 3.5 s. The Chromium host finds the same
streams, but launching rod's downloaded Chromium (no Chrome installed here)
takes 25 s, as it did for the old extractor.

## Next

1. Phone: the same `capture.js` in an Android WebView
   (`addDocumentStartJavaScript` for every frame, a `CastBridge` JavaScript
   interface, `CookieManager` for HttpOnly cookies), sending the result with
   `/play-extracted`. Fyne has no WebView, so this needs either a native
   Android screen in the remote or the move to Wails 3 (stable on desktop
   since v3.0.0; mobile still experimental).
2. macOS: the same script through `WKUserScript` (all frames) and a
   `WKScriptMessageHandler`, replacing the Chromium host and go-rod.
