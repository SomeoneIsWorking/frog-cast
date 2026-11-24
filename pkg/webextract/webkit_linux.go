//go:build !chromium && !android

package webextract

/*
#cgo pkg-config: webkit2gtk-4.1 gtk+-3.0 libsoup-3.0
#include <stdint.h>
#include <stdlib.h>
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include <libsoup/soup.h>

extern void goRun(uintptr_t fn);
extern void goMessage(uintptr_t view, char *json);
extern void goFilterSaved(uintptr_t view, char *err);
extern void goCookies(uintptr_t view, char *cookies);

typedef struct {
	uintptr_t handle;
	GtkWidget *window;
	WebKitWebView *view;
	WebKitWebContext *context;
	WebKitUserContentManager *content;
} View;

static gboolean run_idle(gpointer fn) {
	goRun((uintptr_t)fn);
	return G_SOURCE_REMOVE;
}

static void run_on_main(uintptr_t fn) {
	g_idle_add(run_idle, (gpointer)fn);
}

static void on_message(WebKitUserContentManager *m, WebKitJavascriptResult *r, gpointer data) {
	View *v = data;
	char *text = jsc_value_to_string(webkit_javascript_result_get_js_value(r));
	goMessage(v->handle, text);
	g_free(text);
}

static View *view_new(uintptr_t handle, const char *script, gboolean visible) {
	View *v = g_new0(View, 1);
	v->handle = handle;
	v->context = webkit_web_context_new_ephemeral();
	v->content = webkit_user_content_manager_new();
	WebKitUserScript *us = webkit_user_script_new(script, WEBKIT_USER_CONTENT_INJECT_ALL_FRAMES,
		WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, NULL, NULL);
	webkit_user_content_manager_add_script(v->content, us);
	webkit_user_script_unref(us);
	g_signal_connect(v->content, "script-message-received::cast", G_CALLBACK(on_message), v);
	webkit_user_content_manager_register_script_message_handler(v->content, "cast");
	WebKitWebsitePolicies *policies = webkit_website_policies_new_with_policies("autoplay", WEBKIT_AUTOPLAY_ALLOW, NULL);
	v->view = WEBKIT_WEB_VIEW(g_object_new(WEBKIT_TYPE_WEB_VIEW,
		"web-context", v->context,
		"user-content-manager", v->content,
		"website-policies", policies,
		NULL));
	g_object_unref(policies);
	webkit_web_view_set_is_muted(v->view, TRUE);
	// An offscreen window has no GL context; nothing here needs compositing.
	webkit_settings_set_hardware_acceleration_policy(webkit_web_view_get_settings(v->view),
		WEBKIT_HARDWARE_ACCELERATION_POLICY_NEVER);
	v->window = visible ? gtk_window_new(GTK_WINDOW_TOPLEVEL) : gtk_offscreen_window_new();
	gtk_window_set_default_size(GTK_WINDOW(v->window), 1280, 720);
	gtk_container_add(GTK_CONTAINER(v->window), GTK_WIDGET(v->view));
	gtk_widget_show_all(v->window);
	return v;
}

static void filter_saved(GObject *store, GAsyncResult *res, gpointer data) {
	View *v = data;
	GError *err = NULL;
	WebKitUserContentFilter *filter = webkit_user_content_filter_store_save_finish(
		WEBKIT_USER_CONTENT_FILTER_STORE(store), res, &err);
	if (filter) {
		webkit_user_content_manager_add_filter(v->content, filter);
		webkit_user_content_filter_unref(filter);
	}
	goFilterSaved(v->handle, err ? err->message : NULL);
	if (err) {
		g_error_free(err);
	}
	g_object_unref(store);
}

static void view_block(View *v, const char *dir, const char *list) {
	WebKitUserContentFilterStore *store = webkit_user_content_filter_store_new(dir);
	GBytes *bytes = g_bytes_new(list, strlen(list));
	webkit_user_content_filter_store_save(store, "block", bytes, NULL, filter_saved, v);
	g_bytes_unref(bytes);
}

static void view_load(View *v, const char *uri) {
	webkit_web_view_load_uri(v->view, uri);
}

static const char *view_user_agent(View *v) {
	return webkit_settings_get_user_agent(webkit_web_view_get_settings(v->view));
}

static void cookies_got(GObject *manager, GAsyncResult *res, gpointer data) {
	View *v = data;
	GList *list = webkit_cookie_manager_get_cookies_finish(WEBKIT_COOKIE_MANAGER(manager), res, NULL);
	GString *out = g_string_new(NULL);
	for (GList *l = list; l; l = l->next) {
		SoupCookie *c = l->data;
		g_string_append_printf(out, "%s=%s\n", soup_cookie_get_name(c), soup_cookie_get_value(c));
	}
	g_list_free_full(list, (GDestroyNotify)soup_cookie_free);
	goCookies(v->handle, out->str);
	g_string_free(out, TRUE);
}

static void view_cookies(View *v, const char *uri) {
	WebKitCookieManager *m = webkit_website_data_manager_get_cookie_manager(
		webkit_web_context_get_website_data_manager(v->context));
	webkit_cookie_manager_get_cookies(m, uri, NULL, cookies_got, v);
}

static void view_free(View *v) {
	gtk_widget_destroy(v->window);
	g_object_unref(v->content);
	g_object_unref(v->context);
	g_free(v);
}
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime/cgo"
	"strings"
	"sync/atomic"
	"unsafe"
)

// loopRunning is whether onMain has a GTK main loop: the host app's or RunMainLoop's.
var loopRunning atomic.Bool

// UseHostLoop records that the host app (Wails) runs the GTK main loop.
func UseHostLoop() {
	loopRunning.Store(true)
}

// Available reports whether Extract can run in this process.
func Available() bool {
	return loopRunning.Load()
}

// onMain runs fn on the GTK main loop, which must be running (the app's, or
// RunMainLoop's). Callers must not be on that loop themselves.
func onMain(fn func()) {
	C.run_on_main(C.uintptr_t(cgo.NewHandle(fn)))
}

//export goRun
func goRun(fn C.uintptr_t) {
	h := cgo.Handle(fn)
	defer h.Delete()
	h.Value().(func())()
}

// webView is one extraction's offscreen WebView; its callbacks arrive on the
// GTK main loop and are passed on through channels.
type webView struct {
	c        *C.View
	messages chan Message
	filter   chan error
	cookies  chan map[string]string
}

//export goMessage
func goMessage(handle C.uintptr_t, text *C.char) {
	v := cgo.Handle(handle).Value().(*webView)
	var m Message
	if json.Unmarshal([]byte(C.GoString(text)), &m) != nil {
		return
	}
	select {
	case v.messages <- m:
	default:
	}
}

//export goFilterSaved
func goFilterSaved(handle C.uintptr_t, err *C.char) {
	v := cgo.Handle(handle).Value().(*webView)
	if err != nil {
		v.filter <- fmt.Errorf("content blocker: %s", C.GoString(err))
		return
	}
	v.filter <- nil
}

//export goCookies
func goCookies(handle C.uintptr_t, text *C.char) {
	v := cgo.Handle(handle).Value().(*webView)
	cookies := map[string]string{}
	for _, line := range strings.Split(C.GoString(text), "\n") {
		if name, value, ok := strings.Cut(line, "="); ok {
			cookies[name] = value
		}
	}
	v.cookies <- cookies
}

// Extract loads pageURL in an offscreen WebView (a window when visible, for
// someone to click play) until it plays an HLS stream or ctx ends.
func Extract(ctx context.Context, pageURL string, rules Rules, opts Options) (*Result, error) {
	if !Available() {
		return nil, ErrNoBrowser
	}
	js, err := script(rules)
	if err != nil {
		return nil, err
	}
	blockDir, err := os.MkdirTemp("", "webextract")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(blockDir)

	v := &webView{
		messages: make(chan Message, 256),
		filter:   make(chan error, 1),
		cookies:  make(chan map[string]string, 1),
	}
	handle := cgo.NewHandle(v)
	defer handle.Delete()

	created := make(chan string, 1)
	onMain(func() {
		cjs := C.CString(js)
		defer C.free(unsafe.Pointer(cjs))
		v.c = C.view_new(C.uintptr_t(handle), cjs, C.gboolean(boolInt(opts.Visible)))
		created <- C.GoString(C.view_user_agent(v.c))
		if len(rules.Block) == 0 {
			v.filter <- nil
			return
		}
		dir, list := C.CString(blockDir), C.CString(blockList(rules))
		defer C.free(unsafe.Pointer(dir))
		defer C.free(unsafe.Pointer(list))
		C.view_block(v.c, dir, list)
	})
	userAgent := <-created
	defer onMain(func() { C.view_free(v.c) })

	if err := <-v.filter; err != nil {
		return nil, err
	}
	onMain(func() {
		uri := C.CString(pageURL)
		defer C.free(unsafe.Pointer(uri))
		C.view_load(v.c, uri)
	})

	c, err := listen(ctx, pageURL, v.messages, opts)
	if err != nil {
		return nil, err
	}
	streamURL := c.pick().URL
	onMain(func() {
		uri := C.CString(streamURL)
		defer C.free(unsafe.Pointer(uri))
		C.view_cookies(v.c, uri)
	})
	return c.result(userAgent, <-v.cookies)
}

// RunMainLoop runs a GTK main loop on the calling thread until fn returns,
// for a program without one of its own. Call it from the main goroutine
// with the thread locked (runtime.LockOSThread in init).
func RunMainLoop(fn func()) {
	C.gtk_init(nil, nil)
	loopRunning.Store(true)
	go func() {
		fn()
		onMain(func() { C.gtk_main_quit() })
	}()
	C.gtk_main()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
