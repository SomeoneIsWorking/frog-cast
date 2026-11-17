// Runs at document start in every frame of a page loaded in a plain WebView
// (no devtools) and reports the page's HLS stream, subtitles and anything a
// site rule asks for to the host. The host prepends `const config = {...};`.
(() => {
  "use strict";
  if (window.__castCapture) {
    return;
  }
  window.__castCapture = true;

  // Keep references before page scripts run: they may replace globals.
  const webkitHandler = window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.cast;
  const androidBridge = window.CastBridge;
  const stringify = JSON.stringify.bind(JSON);
  const origFetch = window.fetch;
  const xhrOpen = XMLHttpRequest.prototype.open;
  const xhrSend = XMLHttpRequest.prototype.send;
  const setTimeoutRef = window.setTimeout.bind(window);

  const post = (message) => {
    message.frame = location.href;
    message.top = window === window.top;
    const text = stringify(message);
    try {
      if (webkitHandler) {
        webkitHandler.postMessage(text);
      } else if (androidBridge) {
        androidBridge.postMessage(text);
      }
    } catch (e) {
      // The host went away; nothing to report to.
    }
  };

  const absolute = (url) => {
    try {
      return new URL(url, location.href).href;
    } catch (e) {
      return "";
    }
  };

  const reported = new Set();
  const report = (type, url, extra) => {
    const key = type + " " + url;
    if (!url || reported.has(key)) {
      return;
    }
    reported.add(key);
    post(Object.assign({ type, url, title: document.title, cookies: document.cookie }, extra || {}));
  };

  // A body worth reading: playlists and subtitles are small text, segments
  // are large binaries (sometimes disguised as images).
  const readable = (url, contentType, length) => {
    if (length > 2 * 1024 * 1024) {
      return false;
    }
    const type = (contentType || "").toLowerCase();
    return /mpegurl|vtt|text|json|octet-stream/.test(type) || /\.(m3u8|vtt|txt)(\?|#|$)/i.test(url) || type === "";
  };

  const inspect = (url, contentType, body) => {
    const type = (contentType || "").toLowerCase();
    const head = (body || "").slice(0, 64).trimStart();
    if (head.startsWith("#EXTM3U") || /mpegurl/.test(type)) {
      report("manifest", url, { master: /#EXT-X-STREAM-INF/.test(body || ""), body });
    } else if ((head.startsWith("WEBVTT") || /text\/vtt/.test(type)) && !/#xywh=/.test(body || "")) {
      // (A VTT of image regions is a seek-preview thumbnail track.)
      const name = new URL(url).pathname.split("/").pop() || "";
      report("subtitle", url, { label: name.replace(/\.vtt$/i, ""), body });
    }
    for (const rule of config.bodyRules || []) {
      if (url.includes(rule.url)) {
        const match = new RegExp(rule.regex).exec(body || "");
        if (match && match[1]) {
          report("manifestUrl", absolute(match[1]));
        }
      }
    }
  };

  window.fetch = function (input, init) {
    const url = absolute(typeof input === "string" ? input : (input && input.url) || String(input));
    const result = origFetch.apply(this, arguments);
    result.then((response) => {
      const contentType = response.headers.get("content-type");
      const length = Number(response.headers.get("content-length") || 0);
      if (readable(url, contentType, length)) {
        response.clone().text().then((body) => inspect(url, contentType, body), () => {});
      }
    }, () => {});
    return result;
  };

  XMLHttpRequest.prototype.open = function (method, url) {
    this.__castURL = absolute(url);
    return xhrOpen.apply(this, arguments);
  };
  XMLHttpRequest.prototype.send = function () {
    this.addEventListener("load", () => {
      const contentType = this.getResponseHeader("content-type");
      const length = Number(this.getResponseHeader("content-length") || 0);
      if (!readable(this.__castURL, contentType, length)) {
        return;
      }
      let body = "";
      if (this.responseType === "" || this.responseType === "text") {
        body = this.responseText;
      } else if (this.responseType === "arraybuffer" && this.response) {
        body = new TextDecoder().decode(this.response.slice(0, 2 * 1024 * 1024));
      } else if (this.responseType === "json" && this.response) {
        body = stringify(this.response);
      }
      inspect(this.__castURL, contentType, body);
    });
    return xhrSend.apply(this, arguments);
  };

  // Native HLS (Safari, Android) plays the playlist URL straight from the
  // media element, with no fetch to observe.
  const mediaSource = (url) => {
    if (/\.m3u8(\?|#|$)/i.test(url)) {
      report("manifestUrl", absolute(url));
    }
  };
  const srcDescriptor = Object.getOwnPropertyDescriptor(HTMLMediaElement.prototype, "src");
  Object.defineProperty(HTMLMediaElement.prototype, "src", {
    configurable: true,
    get() {
      return srcDescriptor.get.call(this);
    },
    set(value) {
      mediaSource(String(value));
      srcDescriptor.set.call(this, value);
    },
  });

  // Site rules: elements to click as they appear (play overlays), and inline
  // script text holding the stream URL.
  const clicked = new WeakSet();
  // Clicks wait for the frame's scripts to have run and bound their handlers.
  let loaded = false;
  window.addEventListener("load", () => {
    post({ type: "frame", title: document.title });
  });
  window.addEventListener("load", () => setTimeoutRef(() => {
    loaded = true;
    act();
  }, 1000));
  const act = () => {
    for (const selector of loaded ? config.click || [] : []) {
      for (const el of document.querySelectorAll(selector)) {
        if (!clicked.has(el)) {
          clicked.add(el);
          setTimeoutRef(() => {
            post({ type: "click", url: selector });
            el.click();
          }, 500);
        }
      }
    }
    for (const rule of config.inlineRules || []) {
      if (!location.href.includes(rule.url)) {
        continue;
      }
      for (const script of document.scripts) {
        const match = new RegExp(rule.regex).exec(script.textContent || "");
        if (match && match[1]) {
          report("manifestUrl", absolute(match[1]));
        }
      }
    }
    // Nobody scrolls this view: a lazy player frame would never load.
    for (const frame of document.querySelectorAll('iframe[loading="lazy"]')) {
      frame.loading = "eager";
    }
    for (const media of document.querySelectorAll("video[src], video source[src]")) {
      mediaSource(media.getAttribute("src"));
    }
  };
  new MutationObserver(act).observe(document, { childList: true, subtree: true });
  document.addEventListener("DOMContentLoaded", act);
  post({ type: "frame", title: document.title });
})();
