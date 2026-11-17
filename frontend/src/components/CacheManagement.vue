<template>
  <div class="setting-item">
    <div class="setting-panel items-start! flex">
      <label v-if="libraryStore.sources.length > 1" class="flex items-center gap-2 mb-3">
        <span class="text-gray-400">Instance:</span>
        <select v-model="sourceBase" class="input-field">
          <option v-for="s in libraryStore.sources" :key="s.base" :value="s.base">
            {{ s.name }}
          </option>
        </select>
      </label>
      <div v-if="cacheStats" class="grid grid-cols-2 gap-x-6 gap-y-2 mb-3">
        <span class="text-gray-400">Total Cache:</span>
        <span class="text-white font-medium">{{
          formatBytes(cacheStats.total)
        }}</span>
        <span class="text-gray-400">Transcoded:</span>
        <span class="text-white">{{
          formatBytes(cacheStats.transcoded)
        }}</span>
        <span class="text-gray-400">Raw:</span>
        <span class="text-white">{{
          formatBytes(cacheStats.rawSegments)
        }}</span>
        <span class="text-gray-400">Metadata:</span>
        <span class="text-white">{{
          formatBytes(cacheStats.metadata)
        }}</span>
      </div>
      <div class="flex flex-col gap-2">
        <button
          @click="handleDeleteTranscodedCache"
          class="btn-warning btn-sm w-full"
        >
          <Trash2 class="w-4 h-4" />
          Delete Transcoded Cache
          <span v-if="cacheStats" class="text-xs opacity-75 ml-auto"
            >({{ formatBytes(cacheStats.transcoded) }})</span
          >
        </button>
        <button
          @click="handleDeleteAllVideoCache"
          class="btn-warning btn-sm w-full"
        >
          <Trash2 class="w-4 h-4" />
          Delete All Video Cache
          <span v-if="cacheStats" class="text-xs opacity-75 ml-auto"
            >({{
              formatBytes(
                cacheStats.transcoded + cacheStats.rawSegments
              )
            }})</span
          >
        </button>
        <button @click="handleDeleteAllCache" class="btn-danger btn-sm w-full">
          <Trash2 class="w-4 h-4" />
          Delete All Cache
          <span v-if="cacheStats" class="text-xs opacity-75 ml-auto"
            >({{ formatBytes(cacheStats.total) }})</span
          >
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useConfirm } from "../composables/useConfirm";
import { Trash2 } from "lucide-vue-next";
import { castapi } from "../../wailsjs/go/models";
import { instanceService, type CachePart } from "@/services/instance";
import { useLibraryStore } from "@/stores/library";
import { LOCAL_SOURCE } from "@/services/source";

const { confirm } = useConfirm();
const libraryStore = useLibraryStore();

// The instance whose cache is shown, by base URL ("" is this machine);
// starts on the one being browsed.
const sourceBase = ref(libraryStore.browseSource.base);
const source = computed(
  () =>
    libraryStore.sources.find((s) => s.base === sourceBase.value) ||
    LOCAL_SOURCE
);

const cacheStats = ref<castapi.CacheStats | null>(null);

const loadCacheStats = async () => {
  cacheStats.value = null;
  cacheStats.value = await instanceService.cacheStats(source.value);
};

watch(source, loadCacheStats);

const formatBytes = (bytes: number): string => {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + " " + sizes[i];
};

const clear = (part: CachePart, title: string, message: string, confirmText: string) =>
  confirm({
    title,
    message: `${message} (${source.value.name})`,
    confirmText,
    cancelText: "Cancel",
    variant: "danger",
    onConfirm: async () => {
      cacheStats.value = await instanceService.clearCache(source.value, part);
    },
  });

const handleDeleteAllCache = () =>
  clear(
    "all",
    "Delete All Cache",
    "This will delete all cached files including metadata, video segments, and extraction data. Are you sure?",
    "Delete All"
  );

const handleDeleteTranscodedCache = () =>
  clear(
    "transcoded",
    "Delete Transcoded Cache",
    "This will delete only transcoded video segments, keeping raw segments and metadata. Are you sure?",
    "Delete Transcoded"
  );

const handleDeleteAllVideoCache = () =>
  clear(
    "video",
    "Delete All Video Cache",
    "This will delete all video files (.ts segments) but keep metadata and extraction data. Are you sure?",
    "Delete Videos"
  );

onMounted(loadCacheStats);
</script>
