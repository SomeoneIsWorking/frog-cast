import { defineStore } from "pinia";
import { ref, computed, watch } from "vue";
import { Device, deviceService } from "../services/device";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { castapi, ffmpeg, main, options } from "../../wailsjs/go/models";
import { mediaService } from "@/services/media";
import { useSettingsStore } from "./settings";
import { parseSubtitlePath, buildSubtitlePath } from "@/utils/subtitle";
import { isWebUrl } from "@/utils/file";
import { useHistoryStore } from "./history";
import {
  activeSource,
  isRemoteActive,
  LOCAL_SOURCE,
  sourceKey,
  type Source,
} from "@/services/source";

interface FrontendCastOptions {
  VideoTrack: number;
  AudioTrack: number;
  Bitrate: string;
  SubtitleType: string;
  SubtitlePath: string;
}

export const useCastStore = defineStore("cast", () => {
  const settingsStore = useSettingsStore();

  // State
  const devices = ref<Device[]>([]);
  const selectedDevice = ref<Device | null>(null);
  const selectedMedia = ref<string | null>(null);
  const isLoading = ref(false);
  const isCasting = computed(() => playbackState.value.status !== "STOPPED");
  const error = ref<string | null>(null);
  const ffmpegInfo = ref<ffmpeg.FFmpegInfo | null>(null);
  const trackInfo = ref<castapi.TrackDisplayInfo | null>(null);
  const historyStore = useHistoryStore();

  // Cast targets exposed by the active remote source (when playing a remote
  // item), and the chosen target host on that remote.
  const remoteDevices = ref<castapi.RemoteDevice[]>([]);
  const remoteTargetHost = ref<string>("local");

  // Local playback events only apply when playing locally; remote playback is
  // reflected by polling the remote /state (see startStatePoll).
  EventsOn("playback:state", (state: castapi.PlaybackState) => {
    if (isRemoteActive()) return;
    playbackState.value = state;
  });

  // Poll the active remote instance's playback state while it is the target.
  let statePoll: number | null = null;
  const stopStatePoll = () => {
    if (statePoll !== null) {
      clearInterval(statePoll);
      statePoll = null;
    }
  };
  // Last position the remote reported. Older remotes only update it when the
  // TV sends a status change, so a repeat of the same value while playing means
  // "no news": keep the player bar's own ticking instead of snapping back.
  let lastRemotePosition: number | null = null;
  const applyRemoteState = (next: castapi.PlaybackState) => {
    const unchanged =
      next.status === "PLAYING" &&
      playbackState.value.status === "PLAYING" &&
      next.mediaPath === playbackState.value.mediaPath &&
      next.currentTime === lastRemotePosition;
    lastRemotePosition = next.currentTime;
    playbackState.value = unchanged
      ? { ...next, currentTime: playbackState.value.currentTime }
      : next;
    // The remote cast ended: hand the controls back to this machine.
    if (next.status === "STOPPED") {
      stopStatePoll();
      activeSource.value = LOCAL_SOURCE;
    }
  };
  const startStatePoll = () => {
    stopStatePoll();
    lastRemotePosition = null;
    statePoll = window.setInterval(async () => {
      if (!isRemoteActive()) return;
      try {
        const { RemoteState } = await import("../../wailsjs/go/main/App");
        const s = activeSource.value;
        applyRemoteState(await RemoteState(s.base, s.token));
      } catch {
        /* transient */
      }
    }, 2000);
  };

  // attachToRemotePlayback looks for another frog-cast instance on the LAN
  // that is already casting and, if this machine is idle, takes over its
  // controls (pause/seek/volume/stop route to it). Read-only until the user
  // presses a control, so it never disturbs what is playing.
  const attachToRemotePlayback = async () => {
    if (playbackState.value.status !== "STOPPED") return;
    const { DiscoverCastInstances, RemoteState } = await import(
      "../../wailsjs/go/main/App"
    );
    let instances: castapi.CastInstance[] = [];
    try {
      instances = await DiscoverCastInstances();
    } catch {
      return;
    }
    for (const inst of instances) {
      const source: Source = {
        kind: "remote",
        name: inst.name,
        base: inst.url,
        token: "",
      };
      try {
        const state = await RemoteState(source.base, source.token);
        if (state.status === "STOPPED") continue;
        // Something may have started locally while we were discovering.
        if (playbackState.value.status !== "STOPPED") return;
        activeSource.value = source;
        playbackState.value = state;
        startStatePoll();
        lastRemotePosition = state.currentTime;
        return;
      } catch {
        /* unreachable or needs a token: skip */
      }
    }
  };

  // Playback State
  const playbackState = ref<castapi.PlaybackState>({
    status: "STOPPED",
    mediaPath: "",
    mediaName: "",
    deviceUrl: "",
    deviceName: "",
    currentTime: 0,
    duration: 0,
    volume: 1,
    muted: false,
  });

  // Cast Options
  const castOptions = ref<FrontendCastOptions>();

  // Tracks of whatever is playing on the active source, for the player bar's
  // pickers. Separate from trackInfo, which belongs to the Cast Options panel
  // and may show a different item. The current choice is
  // playbackState.selection; null when the instance doesn't report one.
  const playingTrackInfo = ref<castapi.TrackDisplayInfo | null>(null);
  const playingSelection = computed(
    () => playbackState.value.selection ?? null
  );
  let playingTrackInfoKey = "";
  const loadPlayingTrackInfo = async () => {
    const state = playbackState.value;
    if (state.status === "STOPPED" || !state.mediaPath) {
      playingTrackInfoKey = "";
      playingTrackInfo.value = null;
      return;
    }
    const source = activeSource.value;
    const key = sourceKey(source) + "|" + state.mediaPath;
    if (key === playingTrackInfoKey) return;
    playingTrackInfoKey = key;
    playingTrackInfo.value = null;
    try {
      const { RemoteTrackInfo, GetTrackDisplayInfo } = await import(
        "../../wailsjs/go/main/App"
      );
      const info =
        source.kind === "remote"
          ? await RemoteTrackInfo(source.base, source.token, state.mediaPath)
          : await GetTrackDisplayInfo(state.mediaPath);
      if (playingTrackInfoKey === key) playingTrackInfo.value = info;
    } catch {
      // Leave the pickers disabled; the next media or source change retries.
      if (playingTrackInfoKey === key) playingTrackInfoKey = "";
    }
  };
  watch(
    () => [
      playbackState.value.mediaPath,
      playbackState.value.status === "STOPPED",
      sourceKey(activeSource.value),
    ],
    loadPlayingTrackInfo
  );

  // Computed
  const hasDevices = computed(() => devices.value.length > 0);
  const hasSelectedDevice = computed(() => selectedDevice.value !== null);
  const hasSelectedMedia = computed(() => selectedMedia.value !== null);
  const isReadyToCast = computed(
    () => hasSelectedDevice.value && hasSelectedMedia.value
  );

  // Actions
  const setDevices = (newDevices: Device[]) => {
    devices.value = newDevices;
  };

  const selectDevice = (device: Device) => {
    selectedDevice.value = device;
    // Reset media if needed, or keep it
  };

  const setTrackInfo = (info: castapi.TrackDisplayInfo) => {
    trackInfo.value = info;
    const historyItem = historyStore.items.find(
      (item) => item.path === info.Path
    );

    const historyCastOptions = historyItem?.castOptions;
    if (historyCastOptions) {
      const subtitleItem = parseSubtitlePath(
        historyCastOptions.SubtitlePath
      );
      castOptions.value = {
        VideoTrack: historyCastOptions.VideoTrack,
        AudioTrack: historyCastOptions.AudioTrack,
        Bitrate: historyCastOptions.Bitrate,
        SubtitleType: subtitleItem.type,
        SubtitlePath: subtitleItem.path,
      };
    } else {
      castOptions.value = {
        VideoTrack: 0,
        AudioTrack: 0,
        Bitrate: settingsStore.settings.defaultQuality,
        SubtitleType: info?.NearSubtitle ? "external" : "none",
        SubtitlePath: info?.NearSubtitle || "",
      };
    }
  };

  EventsOn("discovery:complete", () => {
    isLoading.value = false;
  });
  EventsOn("device:found", (device: Device) => {
    // Avoid duplicates
    if (!devices.value.find((d) => d.address === device.address)) {
      setDevices([...devices.value, device]);
    }
  });

  const discoverDevices = async () => {
    isLoading.value = true;
    // Clear existing devices and register event listeners to update UI as devices are found.
    setDevices([]);

    // Trigger backend discovery (returns quickly since discovery is streamed via events)
    await deviceService.discoverDevices();
  };

  // The source that casts the item in the Cast Options panel. Playback moves
  // to it (activeSource) only when that item is cast.
  const optionsSource = ref<Source>(LOCAL_SOURCE);

  // prepareEpisode loads track info for a library item of a given source and,
  // for a remote source, its cast targets. The UI then shows Cast Options.
  const prepareEpisode = async (path: string, source: Source) => {
    if (source.kind === "remote") {
      const { RemoteTrackInfo } = await import("../../wailsjs/go/main/App");
      const [info] = await Promise.all([
        RemoteTrackInfo(source.base, source.token, path),
        loadRemoteTargets(source),
      ]);
      optionsSource.value = source;
      setTrackInfo(info);
    } else {
      const { GetTrackDisplayInfo } = await import("../../wailsjs/go/main/App");
      const info = await GetTrackDisplayInfo(path);
      optionsSource.value = source;
      setTrackInfo(info);
    }
  };

  // prepareFromThisMachine readies a file or web page picked on this machine.
  // A file is always cast from here. A web page is extracted here (a site
  // without a script opens its browser on this screen); when a remote is
  // playing, the extraction goes to it and the options are its tracks, so
  // downloads and the cast happen there.
  const prepareFromThisMachine = async (path: string) => {
    const source =
      isWebUrl(path) && isRemoteActive() ? activeSource.value : LOCAL_SOURCE;
    if (source.kind === "remote") return await preparePage(path, source);
    const { GetTrackDisplayInfo } = await import("../../wailsjs/go/main/App");
    const info = await GetTrackDisplayInfo(path);
    optionsSource.value = source;
    setTrackInfo(info);
  };

  // preparePage readies a web page to cast from a remote: its tracks as the
  // remote has them, extracted here and sent when it can't extract the page.
  const preparePage = async (path: string, source: Source) => {
    const { RemoteOpenPage } = await import("../../wailsjs/go/main/App");
    const [info] = await Promise.all([
      RemoteOpenPage(source.base, source.token, path),
      loadRemoteTargets(source),
    ]);
    optionsSource.value = source;
    setTrackInfo(info);
  };

  // prepareHistoryItem readies an entry of a source's history to cast again
  // from that source.
  const prepareHistoryItem = async (path: string, source: Source) => {
    if (source.kind === "local") return await prepareFromThisMachine(path);
    if (isWebUrl(path)) return await preparePage(path, source);
    return await prepareEpisode(path, source);
  };

  // loadRemoteTargets lists a remote's cast targets for the Cast Options
  // panel, defaulting to its first real device (Chromecast), else the remote
  // itself.
  const loadRemoteTargets = async (s: Source) => {
    const { RemoteDevices } = await import("../../wailsjs/go/main/App");
    const devs = await RemoteDevices(s.base, s.token);
    remoteDevices.value = devs;
    const def = devs.find((d) => d.host !== "local") || devs[0];
    remoteTargetHost.value = def ? def.host : "local";
  };

  const startCasting = async (media: string) => {
    if (!castOptions.value) return;
    selectedMedia.value = media;

    const subtitlePath = buildSubtitlePath(
      castOptions.value.SubtitleType,
      castOptions.value.SubtitlePath
    );

    const s = optionsSource.value;
    if (s.kind === "remote") {
      const { RemotePlay } = await import("../../wailsjs/go/main/App");
      playbackState.value = await RemotePlay(
        s.base,
        s.token,
        media,
        remoteTargetHost.value || "local",
        {
          videoTrack: castOptions.value.VideoTrack,
          audioTrack: castOptions.value.AudioTrack,
          subtitlePath,
          quality: castOptions.value.Bitrate,
          // This controller's subtitle look is the default for casts it starts.
          subtitle: settingsStore.settings
            ? {
                fontSize: settingsStore.settings.subtitleFontSize,
                bold: settingsStore.settings.subtitleBold,
                italic: settingsStore.settings.subtitleItalic,
                burnIn: settingsStore.settings.subtitleBurnIn,
              }
            : undefined,
        }
      );
      activeSource.value = s;
      startStatePoll();
      lastRemotePosition = playbackState.value.currentTime;
      // Only this machine's history updates by event.
      if (historyStore.source.base === s.base) historyStore.loadHistory();
    } else {
      if (!selectedDevice.value) return;
      stopStatePoll();
      activeSource.value = LOCAL_SOURCE;
      const backendOptions: options.CastOptions = {
        VideoTrack: castOptions.value.VideoTrack,
        AudioTrack: castOptions.value.AudioTrack,
        Bitrate: castOptions.value.Bitrate,
        SubtitlePath: subtitlePath,
      };
      playbackState.value = await mediaService.castToDevice(
        selectedDevice.value.host,
        media,
        backendOptions
      );
    }
  };

  // changeTracks switches what the running cast plays, on whichever instance
  // is casting. The TV reloads at the current position.
  const changeTracks = async (change: castapi.TrackChange) => {
    const state = await mediaService.changeTracks(change);
    if (isRemoteActive()) lastRemotePosition = state.currentTime;
    playbackState.value = state;
  };

  const checkFFmpeg = async () => {
    const { GetFFmpegInfo } = await import("../../wailsjs/go/main/App");
    ffmpegInfo.value = await GetFFmpegInfo(true);
  };

  // updateLiveSubtitleSettings applies subtitle tweaks (timing offset, bold,
  // italic, font size) to the running cast without recasting. The casting
  // instance owns the running cast's subtitles and keeps the timing offset
  // per episode; size and style also become this controller's defaults for
  // casts it starts.
  const updateLiveSubtitleSettings = async (overrides: {
    delaySeconds?: number;
    bold?: boolean;
    italic?: boolean;
    fontSize?: number;
  }) => {
    const settings = settingsStore.settings;
    const current = playingSelection.value;

    if (settings && (overrides.bold !== undefined || overrides.italic !== undefined || overrides.fontSize !== undefined)) {
      const next: main.Settings = { ...settings };
      if (overrides.bold !== undefined) next.subtitleBold = overrides.bold;
      if (overrides.italic !== undefined) next.subtitleItalic = overrides.italic;
      if (overrides.fontSize !== undefined)
        next.subtitleFontSize = overrides.fontSize;
      await settingsStore.saveSettings(next);
    }

    // Start from what the cast has now (this controller's settings only when
    // the instance doesn't report it). An empty Path keeps the subtitle
    // source: restyling never changes which subtitles are shown.
    const opts: options.SubtitleCastOptions = {
      Path: "",
      BurnIn: current?.burnIn ?? settings?.subtitleBurnIn ?? false,
      FontSize:
        overrides.fontSize ?? current?.fontSize ?? settings?.subtitleFontSize ?? 24,
      IgnoreClosedCaptions:
        current?.ignoreClosedCaptions ?? settings?.ignoreClosedCaptions ?? false,
      DelaySeconds: overrides.delaySeconds ?? current?.delaySeconds ?? 0,
      Bold: overrides.bold ?? current?.bold ?? settings?.subtitleBold ?? false,
      Italic: overrides.italic ?? current?.italic ?? settings?.subtitleItalic ?? false,
    };

    await mediaService.updateSubtitleSettings(opts);
    // A remote's next poll is up to 2s away; show the change now so the next
    // tweak builds on it.
    if (current) {
      playbackState.value = {
        ...playbackState.value,
        selection: {
          ...current,
          fontSize: opts.FontSize,
          delaySeconds: opts.DelaySeconds,
          bold: opts.Bold,
          italic: opts.Italic,
        },
      };
    }
  };

  return {
    // State
    devices,
    selectedDevice,
    selectedMedia,
    isLoading,
    isCasting,
    error,
    ffmpegInfo,
    playbackState,
    castOptions,
    trackInfo,
    playingTrackInfo,
    playingSelection,
    remoteDevices,
    remoteTargetHost,

    // Computed
    hasDevices,
    hasSelectedDevice,
    hasSelectedMedia,
    isReadyToCast,

    // Actions
    setDevices,
    selectDevice,
    setTrackInfo,
    discoverDevices,
    attachToRemotePlayback,
    prepareEpisode,
    prepareFromThisMachine,
    optionsSource,
    prepareHistoryItem,
    startCasting,
    checkFFmpeg,
    updateLiveSubtitleSettings,
    changeTracks,
  };
});
