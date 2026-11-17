<template>
  <div class="relative inline-block" ref="containerRef">
    <button
      @click="showPopover = !showPopover"
      class="btn-icon focus:outline-none"
      title="Tracks"
      @keydown.esc="showPopover = false"
    >
      <Layers :size="20" />
    </button>

    <transition
      enter-active-class="transition duration-150 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition duration-100 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div
        v-if="showPopover"
        class="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 p-3 bg-gray-800 border border-gray-700 rounded-xl shadow-2xl z-50 w-[360px]"
      >
        <p v-if="!selection" class="text-sm text-gray-400">
          This instance doesn't report what it's playing.
        </p>
        <p v-else-if="!info" class="text-sm text-gray-400">Loading tracks…</p>
        <div
          v-else
          class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-2 items-center text-sm"
        >
          <label class="text-xs font-semibold text-gray-400 uppercase tracking-wider">Video</label>
          <select
            :value="selection.videoTrack"
            :disabled="busy"
            @change="apply({ videoTrack: numberOf($event) })"
            class="bg-gray-700 text-white rounded-md p-1.5"
          >
            <option v-for="t in info.VideoTracks" :key="t.Index" :value="t.Index">
              {{ t.Index }} – {{ t.Resolution || t.Codecs || "Unknown" }}
            </option>
          </select>

          <label class="text-xs font-semibold text-gray-400 uppercase tracking-wider">Quality</label>
          <select
            :value="selection.quality"
            :disabled="busy"
            @change="apply({ quality: stringOf($event) })"
            class="bg-gray-700 text-white rounded-md p-1.5"
          >
            <option v-for="q in qualityOptions" :key="q.value" :value="q.value">
              {{ q.label }}
            </option>
          </select>

          <template v-if="info.AudioTracks.length > 0">
            <label class="text-xs font-semibold text-gray-400 uppercase tracking-wider">Audio</label>
            <select
              :value="selection.audioTrack"
              :disabled="busy"
              @change="apply({ audioTrack: numberOf($event) })"
              class="bg-gray-700 text-white rounded-md p-1.5"
            >
              <option v-for="t in info.AudioTracks" :key="t.Index" :value="t.Index">
                {{ t.Index }} – {{ t.Language || "Unknown" }}
              </option>
            </select>
          </template>

          <label class="text-xs font-semibold text-gray-400 uppercase tracking-wider">Subtitles</label>
          <select
            :value="selection.subtitlePath"
            :disabled="busy"
            @change="apply({ subtitlePath: stringOf($event) })"
            class="bg-gray-700 text-white rounded-md p-1.5"
          >
            <option v-for="s in subtitleOptions" :key="s.path" :value="s.path">
              {{ s.label }}
            </option>
          </select>

          <p v-if="busy" class="col-span-2 text-xs text-gray-400">
            Switching… the TV reloads at the same spot.
          </p>
        </div>

        <div class="absolute top-full left-1/2 -translate-x-1/2 border-8 border-transparent border-t-gray-800"></div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { Layers } from "lucide-vue-next";
import { useToast } from "vue-toastification";
import { useCastStore } from "@/stores/cast";
import { qualityOptions } from "@/data/qualityOptions";
import type { castapi } from "../../wailsjs/go/models";

const castStore = useCastStore();
const toast = useToast();
const showPopover = ref(false);
const containerRef = ref<HTMLElement | null>(null);
const busy = ref(false);

const selection = computed(() => castStore.playingSelection);
const info = computed(() => castStore.playingTrackInfo);

const baseName = (path: string) => path.split(/[\\/]/).pop() || path;

// Subtitle sources on the casting instance: none, each embedded track, and
// the external files it knows about (the current one and the one beside the
// media). Picking a file elsewhere isn't offered: it must exist over there.
const subtitleOptions = computed(() => {
  const out: { path: string; label: string }[] = [];
  for (const s of info.value?.SubtitleTracks ?? []) {
    if (s.Path !== "external") out.push({ path: s.Path, label: s.Label });
  }
  const external = new Set<string>();
  const current = selection.value?.subtitlePath ?? "";
  if (current.startsWith("external:")) external.add(current);
  if (info.value?.NearSubtitle) external.add("external:" + info.value.NearSubtitle);
  for (const path of external) {
    out.push({ path, label: "External: " + baseName(path.slice("external:".length)) });
  }
  return out;
});

const numberOf = (e: Event) => Number((e.target as HTMLSelectElement).value);
const stringOf = (e: Event) => (e.target as HTMLSelectElement).value;

const apply = async (change: castapi.TrackChange) => {
  busy.value = true;
  try {
    await castStore.changeTracks(change);
  } catch (err) {
    toast.error(`Couldn't switch tracks: ${err}`);
  } finally {
    busy.value = false;
  }
};

const handleClickOutside = (event: MouseEvent) => {
  if (containerRef.value && !containerRef.value.contains(event.target as Node)) {
    showPopover.value = false;
  }
};

onMounted(() => document.addEventListener("mousedown", handleClickOutside));
onUnmounted(() => document.removeEventListener("mousedown", handleClickOutside));
</script>
