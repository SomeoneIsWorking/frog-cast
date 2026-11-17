import { defineStore } from "pinia";
import { ref, computed, watch } from "vue";
import { main } from "../../wailsjs/go/models";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { instanceService, type HistoryEntry } from "@/services/instance";
import { useLibraryStore } from "./library";

// The history of the instance being browsed (the library's source): what it
// cast, newest first. This machine's updates arrive by event.
export const useHistoryStore = defineStore("history", () => {
  const libraryStore = useLibraryStore();
  const items = ref<HistoryEntry[]>([]);
  const isLoading = ref(false);
  const error = ref<string | null>(null);

  const source = computed(() => libraryStore.browseSource);
  const hasHistory = computed(() => items.value.length > 0);

  EventsOn("history:updated", (newItem: main.HistoryItem) => {
    if (source.value.kind === "remote") return;
    items.value = items.value.filter((item) => item.path !== newItem.path);
    items.value.unshift({
      path: newItem.path,
      name: newItem.name,
      timestamp: newItem.timestamp,
      castOptions: newItem.castOptions,
    });
  });

  const loadHistory = async () => {
    isLoading.value = true;
    error.value = null;
    try {
      items.value = await instanceService.history(source.value);
    } catch (e) {
      error.value = String(e);
      items.value = [];
    } finally {
      isLoading.value = false;
    }
  };

  watch(source, loadHistory);

  const removeItem = async (path: string) => {
    await instanceService.removeHistory(source.value, path);
    items.value = items.value.filter((item) => item.path !== path);
  };

  const clearAll = async () => {
    await instanceService.removeHistory(source.value, "");
    items.value = [];
  };

  return {
    items,
    isLoading,
    error,
    source,
    hasHistory,
    loadHistory,
    removeItem,
    clearAll,
  };
});
