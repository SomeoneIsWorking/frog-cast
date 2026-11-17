import { castapi, options } from "../../wailsjs/go/models";
import type { Source } from "./source";

// instanceService reaches one instance's history, media cache and web page
// downloads: this machine's through its bindings, a remote's over its HTTP
// API. Both answer in the remote API's shapes.

export interface HistoryEntry {
  path: string;
  name: string;
  timestamp: string;
  castOptions?: options.CastOptions;
}

export type CachePart = "transcoded" | "video" | "all";

export const instanceService = {
  async history(s: Source): Promise<HistoryEntry[]> {
    if (s.kind === "remote") {
      const { RemoteHistory } = await import("../../wailsjs/go/main/App");
      return await RemoteHistory(s.base, s.token);
    }
    const { GetHistory } = await import("../../wailsjs/go/main/App");
    return (await GetHistory()).map(
      ({ path, name, timestamp, castOptions }) => ({
        path,
        name,
        timestamp,
        castOptions,
      })
    );
  },

  // removeHistory removes one entry; "" clears the history.
  async removeHistory(s: Source, path: string): Promise<void> {
    if (s.kind === "remote") {
      const { RemoteRemoveHistory } = await import("../../wailsjs/go/main/App");
      return await RemoteRemoveHistory(s.base, s.token, path);
    }
    const { RemoveFromHistory, ClearHistory } = await import(
      "../../wailsjs/go/main/App"
    );
    return path === "" ? await ClearHistory() : await RemoveFromHistory(path);
  },

  async cacheStats(s: Source): Promise<castapi.CacheStats> {
    if (s.kind === "remote") {
      const { RemoteCacheStats } = await import("../../wailsjs/go/main/App");
      return await RemoteCacheStats(s.base, s.token);
    }
    const { GetCacheStats } = await import("../../wailsjs/go/main/App");
    return await GetCacheStats();
  },

  async clearCache(s: Source, part: CachePart): Promise<castapi.CacheStats> {
    if (s.kind === "remote") {
      const { RemoteClearCache } = await import("../../wailsjs/go/main/App");
      return await RemoteClearCache(s.base, s.token, part);
    }
    const app = await import("../../wailsjs/go/main/App");
    const clear = {
      transcoded: app.DeleteTranscodedCache,
      video: app.DeleteAllVideoCache,
      all: app.ClearCache,
    }[part];
    await clear();
    return await app.GetCacheStats();
  },

  async downloads(s: Source): Promise<castapi.PageDownload[]> {
    if (s.kind === "remote") {
      const { RemoteDownloads } = await import("../../wailsjs/go/main/App");
      return await RemoteDownloads(s.base, s.token);
    }
    const { Downloads } = await import("../../wailsjs/go/main/App");
    return await Downloads();
  },

  async setDownload(
    s: Source,
    req: castapi.DownloadRequest
  ): Promise<castapi.DownloadStatus> {
    if (s.kind === "remote") {
      const { RemoteSetDownload } = await import("../../wailsjs/go/main/App");
      return await RemoteSetDownload(s.base, s.token, req);
    }
    const { SetDownload } = await import("../../wailsjs/go/main/App");
    return await SetDownload(req);
  },

  async download(
    s: Source,
    url: string,
    type: string,
    track: number
  ): Promise<castapi.DownloadStatus> {
    if (s.kind === "remote") {
      const { RemoteDownload } = await import("../../wailsjs/go/main/App");
      return await RemoteDownload(s.base, s.token, url, type, track);
    }
    const { Download } = await import("../../wailsjs/go/main/App");
    return await Download(url, type, track);
  },
};
