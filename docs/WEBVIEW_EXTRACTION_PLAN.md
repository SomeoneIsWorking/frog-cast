# WebView extraction (no devtools, no Chrome)

Status: step 1 built (2026-11-17): `pkg/webextract` and `cmd/webextract`
extract on Linux through WebKitGTK. The app still extracts through Chrome
(`pkg/extractor`, go-rod); steps 2 and 3 are open.

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
  overlays), and regexes finding the stream URL in a response body or in a
  frame's inline scripts.

It also switches lazy iframes to eager, since nobody scrolls the view. Messages
go to `window.webkit.messageHandlers.cast` (WebKit) or `window.CastBridge`
(an Android WebView).

The Linux host (`webkit_linux.go`) loads the page in an offscreen
`WebKitWebView` on the GTK main loop, with an ephemeral context, autoplay
allowed, sound muted, no GPU compositing, and a content blocker for the
rule's blocked URLs. It picks a master playlist, else a URL found in page
text, else a media playlist, and keeps listening briefly for subtitles. It
returns the playlist, the subtitles, the Referer (the requesting frame), the
Origin when cross-origin, the WebView's User-Agent and the cookies WebKit
holds for the playlist URL.

Try a site with
`go run ./cmd/webextract -url <page> -click <selector> -block <substring>`.
It prints every message and the result, and `-json` writes the result.

## Results (2026-11-17, from Fedora)

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
not attempted. The Lua script for that site also still matches the old
`.mom` domain.

## Next

1. Use it in the app on Linux: run `Extract` on the app's GTK main loop
   (Wails v2 runs one) in `RemoteManager.doExtraction`, with site rules in
   place of the Lua scripts (`match`, `click`, `block`, `bodyRules`,
   `inlineRules`). A page without rules opens visibly for someone to click
   play. Then drop go-rod.
2. Phone: the same `capture.js` in an Android WebView
   (`addDocumentStartJavaScript` for every frame, a `CastBridge` JavaScript
   interface, `CookieManager` for HttpOnly cookies), sending the result with
   `/play-extracted`. Fyne has no WebView, so this needs either a native
   Android screen in the remote or the move to Wails 3 (stable on desktop
   since v3.0.0; mobile still experimental).
3. macOS: the same script through `WKUserScript` (all frames) and a
   `WKScriptMessageHandler`.
