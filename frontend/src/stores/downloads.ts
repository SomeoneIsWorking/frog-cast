import { defineStore } from "pinia";
import { ref } from "vue";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import {
  GetDownloadStatus,
  RemoteDownload,
  RemoteSetDownload,
  StartDownload,
  StopDownload,
} from "../../wailsjs/go/main/App";
import { remote } from "wailsjs/go/models";
import { useCastStore } from "./cast";

// A web page track's download into the cache of the instance that casts it.
export interface TrackDownload {
  URL: string;
  MediaType: string;
  Track: number;
  Status: string;
  Done: number;
  Total: number;
}

const key = (url: string, mediaType: string, track: number) =>
  `${url}|${mediaType}|${track}`;

// Downloads run on the source of the Cast Options panel: this machine, or the
// remote instance the page will be cast from. This machine reports progress by
// event; a remote is polled while a download runs.
export const useDownloadsStore = defineStore("downloads", () => {
  const downloads = ref<Record<string, TrackDownload>>({});
  const polls = new Map<string, number>();

  const put = (
    url: string,
    mediaType: string,
    track: number,
    status: string,
    done: number,
    total: number,
  ) => {
    downloads.value[key(url, mediaType, track)] = {
      URL: url,
      MediaType: mediaType,
      Track: track,
      Status: status,
      Done: done,
      Total: total,
    };
  };
  const putSegments = (
    url: string,
    mediaType: string,
    track: number,
    status: string,
    segments: boolean[],
  ) =>
    put(
      url,
      mediaType,
      track,
      status,
      segments.filter((s) => s).length,
      segments.length,
    );

  EventsOn("download:progress", (data: remote.DownloadStatus) => {
    if (useCastStore().optionsSource.kind === "remote") return;
    putSegments(
      data.URL,
      data.MediaType,
      data.Track,
      data.Status,
      data.Segments,
    );
  });

  const loadTrackProgress = async (
    url: string,
    mediaType: string,
    track: number,
  ) => {
    const source = useCastStore().optionsSource;
    if (source.kind === "remote") {
      const s = await RemoteDownload(
        source.base,
        source.token,
        url,
        mediaType,
        track,
      );
      put(url, mediaType, track, s.status, s.done, s.total);
    } else {
      const s = await GetDownloadStatus(url, mediaType, track);
      putSegments(url, mediaType, track, s.Status, s.Segments);
    }
  };

  const poll = (url: string, mediaType: string, track: number) => {
    const k = key(url, mediaType, track);
    if (polls.has(k)) return;
    const id = window.setInterval(async () => {
      try {
        await loadTrackProgress(url, mediaType, track);
      } finally {
        if (downloads.value[k]?.Status !== "INPROGRESS") {
          window.clearInterval(id);
          polls.delete(k);
        }
      }
    }, 2000);
    polls.set(k, id);
  };

  const setDownload = async (
    url: string,
    mediaType: string,
    track: number,
    start: boolean,
  ) => {
    const source = useCastStore().optionsSource;
    if (source.kind === "remote") {
      const s = await RemoteSetDownload(source.base, source.token, {
        url,
        type: mediaType,
        track,
        start,
      });
      put(url, mediaType, track, s.status, s.done, s.total);
      if (start) poll(url, mediaType, track);
    } else if (start) {
      await StartDownload(url, mediaType, track);
    } else {
      await StopDownload(url, mediaType, track);
    }
  };

  const getDownloadState = (url: string, mediaType: string, track: number) =>
    downloads.value[key(url, mediaType, track)];

  return {
    downloads,
    getDownloadState,
    loadTrackProgress,
    startDownload: (url: string, mediaType: string, track: number) =>
      setDownload(url, mediaType, track, true),
    stopDownload: (url: string, mediaType: string, track: number) =>
      setDownload(url, mediaType, track, false),
  };
});
