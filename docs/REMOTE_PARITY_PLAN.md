# Remote parity plan

**Goal:** a controller (the desktop app attached to another instance, or the
mobile app) can do everything to a running cast that the instance that started
it can: pause, seek, volume, **and** switch the audio track, video track or
quality, and the subtitle source. Local and remote go through the same UI and
the same backend calls, with the only difference being which instance receives
them.

Status: planned, not started (written 2026-11-16).

## Where things stand

What works when the desktop app attaches to a cast on another instance (for
example the Mac attaching to fedora):

- It finds the cast on startup and shows it in the player bar ("via fedora").
- Pause, resume, seek, volume and stop are sent to that instance.
- Subtitle size, bold/italic and timing changes work (`POST /subtitle`).

What doesn't:

- **The attached app doesn't know what's playing.** `/state` (`PlaybackState`)
  reports title, position and volume, but not the chosen video track, audio
  track, quality or subtitle source.
- **No track info for the playing item.** Track and subtitle lists
  (`trackInfo`) are only loaded by `prepareEpisode()` before a cast starts, so
  an attached app has nothing to pick from.
- **No way to change tracks mid-playback, local or remote.** The only way to
  change the audio track, video track or quality today is to start a new cast
  from the Cast Options panel, which starts from 0:00
  (`LoadOptions.StartTime` is hard-coded to 0 in `CastToDevice`).
- The subtitle *source* can already change live on the server side
  (`UpdateSubtitleSettings` takes a `Path` and re-sends the VTT), but no UI
  offers it during playback.

## Phase 0: two bugs to fix first (small)

1. **Changing subtitle settings jumps the video back.** `UpdateSubtitleSettings`
   re-seeks to `playbackState.CurrentTime`, which is the last position the TV
   *reported* and can be minutes old. Use `extrapolatedPosition()` (added with
   the attach work) under `a.mu`.
2. **Restyling subtitles while attached turns them off.** An attached app has
   no `castOptions`, so `updateLiveSubtitleSettings` in `stores/cast.ts` sends
   `Path: "none"` with every style change. Fix: the app remembers the current
   cast's subtitle source (`App.subtitlePath`, set in `CastToDevice`), and an
   empty `Path` in an update means "keep the current source". The frontend
   sends `""` when it doesn't know the source.

## Phase 1: `/state` says what's playing

- Add the active selection to `PlaybackState` (both `castapi.PlaybackState` and
  the desktop one, which are the same type): `videoTrack`, `audioTrack`,
  `subtitlePath`, `quality`, plus subtitle style (`fontSize`, `bold`,
  `italic`, `delaySeconds`, `burnIn`).
- The `App` keeps the `StreamOptions` the current cast was started with, and
  updates them on every live change (subtitle update, track change).
- The local `playback:state` event carries the same fields, so the UI reads
  one shape everywhere.
- Older instances simply omit the fields; the client treats a missing
  selection as unknown and disables the track pickers.

## Phase 2: load track info for whatever is playing

- When the cast store attaches, or when `state.mediaPath` changes, call
  `RemoteTrackInfo(mediaPath)` (or `GetTrackDisplayInfo` locally) and fill
  `trackInfo`.
- Build `castOptions` from `state` (Phase 1), not from the controller's
  history, so the pickers show what is actually playing.
- Check `/track-info` for web URLs: it goes through `RemoteManager.GetMedia`,
  which is cached on the casting instance while it plays, so it shouldn't
  re-extract. Confirm this with a test before relying on it.

## Phase 3: change tracks mid-playback

- **New backend call `App.ChangeTracks(opts)`**, exposed as
  `POST /tracks {videoTrack, audioTrack, quality, subtitlePath}`. It re-casts
  the same media to the same device with the new options, starting at the
  current position (extrapolated), then restores the paused/playing state.
  Volume is device-level and survives on its own.
  - Factor the handler-building half of `CastToDevice` out so both use it, and
    give `Load` a real `StartTime`.
  - Expect a 1–3 second reload on the TV. Acceptable; there's no seamless
    audio-switch path through the default receiver for transcoded HLS.
- **Subtitle source changes pick the cheaper path:**
  - external → external, or turning external subtitles on/off: live, through
    `UpdateSubtitleSettings` (no re-cast).
  - anything involving burn-in: through `ChangeTracks`, since the video has
    to be re-transcoded.
- `castapi.Client.ChangeTracks`, a Wails binding `RemoteChangeTracks`, and a
  `mediaService.changeTracks()` that routes local/remote like the other
  controls in `services/media.ts`.

## Phase 4: UI

- A **Tracks** popover on the player bar, next to the subtitle and volume
  ones: video track/quality, audio track, subtitle source. It shows the current
  selection from `state` and applies changes via Phase 3.
- The same popover serves local casts, so the local app also gains mid-playback
  track switching.
- The mobile app (`cmd/mobile`) gets the same controls through `castapi`.

## Phase 5: who owns subtitle preferences

Today, subtitle style is saved in the *controller's* settings, and the
per-episode timing offset lives in the controller's `localStorage` keyed by
media path. With several controllers that drifts.

Decision: **the casting instance owns everything about a running cast.** It
stores the per-episode timing offset with its history entry and applies it on
every cast, whichever device started it. A controller's own settings only
provide defaults for casts that controller starts.

## Testing

- Go: unit test that `/state` includes the selection fields, and that
  `ChangeTracks` passes the right `StartTime` (fake `application`).
- Manual, as with the attach work: run the Mac in `wails dev`, attach to
  fedora, and change audio track, quality, and subtitle source both ways
  (external↔none, external↔burn-in). Check the TV resumes at the same spot
  and the pickers show the new selection.
- Don't restart fedora's instance while something is being watched; deploy
  between films.

## Out of scope

- Push updates instead of polling `/state` every 2 seconds (an SSE `/events`
  stream would be nicer, but polling is fine at this scale).
- Several controllers changing tracks at the same moment: last write wins.
