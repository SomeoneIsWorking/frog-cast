# Lua site scripts

A site script adapts extraction (`pkg/extractor`, on `pkg/webextract`) to one
site. Scripts live in the config directory under `scripts/`
(`~/.config/frog-cast/scripts/` on Linux,
`~/Library/Application Support/frog-cast/scripts/` on macOS); the extractor
uses the first whose `match_patterns` matches the page.

A page with a script loads hidden and must reach its stream by itself. A page
without one opens in a window for someone to click play, and only from the
machine running the app.

## Format

Every global is optional except `match_patterns`.

```lua
-- Pages this script handles. * is a wildcard; a pattern is tried against the
-- full URL and against host+path without "www.".
match_patterns = {
  "example.com/*",
  "example.*/*",          -- any TLD
}

-- URL substrings never loaded: anti-debugger, pop-under and ad scripts.
block_urls = {
  "disable-devtool",
}

-- CSS selectors clicked, in any frame, once each appears after its frame has
-- loaded. The click is element.click(), so it must hit the element whose
-- handler starts playback, not an overlay with pointer-events: none.
click = { ".play-overlay img" }

-- For a stream URL the page never requests itself: response bodies and frame
-- HTML whose URL contains pattern go to handler, which returns the stream
-- URL (relative is fine) or nil.
intercepts = {
  {
    pattern = "/api/source",
    handler = function(url, body)
      local data = json_decode(body)
      if data and data.file then
        return data.file
      end
      return nil
    end
  }
}
```

Handlers may call `log(text)` and `json_decode(text)` (nil plus an error on
bad JSON).

## What the extractor does

`capture.js` runs in every frame, cross-origin players included, and reports
every HLS playlist (recognised by content, so `.txt` and `.png` disguises
don't matter) and WebVTT subtitle the page loads. Most sites need only a
`click`: the player then requests its playlist and the extractor sees it.
`intercepts` are for players that fail to play, or sites that hand the URL
over in an API response. There is no need to navigate into a player iframe.

The stream is the master playlist when one loads, else an intercepted URL,
else the first media playlist. Requests for it replay the requesting frame
as `Referer` (and `Origin` when cross-origin), the browser's User-Agent and
its cookies for the stream URL.

## Writing one

1. Run the extractor without a script: it opens the page and prints every
   frame, playlist and subtitle it sees.

   ```bash
   go run ./cmd/extract "https://example.com/film/"
   ```

   Click play by hand and note which frame requested the playlist. If it
   was found, a script only needs to automate the click.
2. Find the element that starts playback: the frame's HTML (`curl` the frame
   URL from the trace with its parent as Referer) or the browser's inspector.
   If the click lands on an element with `pointer-events: none`, target the
   child or parent that has the handler.
3. Save the script and run the same command: it reports `Found Lua script`,
   each `click`, and what was found.
4. Check that the stream survives the cast pipeline (segment download,
   transcode): `-segments <dir>` writes the playlists a Chromecast would get
   and the first segment of each track.

   ```bash
   go run ./cmd/extract -segments ./out "https://example.com/film/"
   ffprobe out/video_segment_0.ts
   ```

## When it fails

- No `Found Lua script`: `match_patterns` doesn't match the URL.
- `no HLS stream ... deadline exceeded`: the trace shows whether the click
  ran (`click` lines) and which frames loaded. A frame that never appears is
  often blocked by a `block_urls` entry or waits for a different click.
- `loading ...: status code 403/404`: the CDN refused Go's request with the
  replayed headers. fastplay.mom refuses every client but the browser.
