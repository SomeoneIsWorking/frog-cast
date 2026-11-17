import { defineStore } from "pinia";
import { ref } from "vue";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { castapi, remote } from "../../wailsjs/go/models";
import { instanceService } from "@/services/instance";
import { useCastStore } from "./cast";

const key = (url: string, mediaType: string, track: number) =>
  `${url}|${mediaType}|${track}`;

// The Cast Options panel's track downloads, on the panel's source: this
// machine, or the remote instance the page will be cast from. This machine
// reports progress by event; a remote is polled while a download runs.
export const useDownloadsStore = defineStore("downloads", () => {
  const downloads = ref<Record<string, castapi.DownloadStatus>>({});
  const polls = new Map<string, number>();

  EventsOn("download:progress", (data: remote.DownloadStatus) => {
    if (useCastStore().optionsSource.kind === "remote") return;
    downloads.value[key(data.URL, data.MediaType, data.Track)] = {
      status: data.Status,
      done: data.Segments.filter((s) => s).length,
      total: data.Segments.length,
    };
  });

  const loadTrackProgress = async (
    url: string,
    mediaType: string,
    track: number
  ) => {
    downloads.value[key(url, mediaType, track)] = await instanceService.download(
      useCastStore().optionsSource,
      url,
      mediaType,
      track
    );
  };

  const poll = (url: string, mediaType: string, track: number) => {
    const k = key(url, mediaType, track);
    if (polls.has(k)) return;
    const id = window.setInterval(async () => {
      try {
        await loadTrackProgress(url, mediaType, track);
      } finally {
        if (downloads.value[k]?.status !== "INPROGRESS") {
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
    start: boolean
  ) => {
    const source = useCastStore().optionsSource;
    downloads.value[key(url, mediaType, track)] =
      await instanceService.setDownload(source, {
        url,
        type: mediaType,
        track,
        start,
      });
    if (start && source.kind === "remote") poll(url, mediaType, track);
  };

  const getDownloadState = (url: string, mediaType: string, track: number) =>
    downloads.value[key(url, mediaType, track)];

  return {
    getDownloadState,
    loadTrackProgress,
    startDownload: (url: string, mediaType: string, track: number) =>
      setDownload(url, mediaType, track, true),
    stopDownload: (url: string, mediaType: string, track: number) =>
      setDownload(url, mediaType, track, false),
  };
});
