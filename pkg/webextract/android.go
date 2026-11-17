//go:build android

package webextract

/*
#include <jni.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

static jclass bridge_class;

// take_exception clears a pending Java exception and describes it.
static char *take_exception(JNIEnv *env) {
	jthrowable t = (*env)->ExceptionOccurred(env);
	if (!t) {
		return NULL;
	}
	(*env)->ExceptionClear(env);
	jclass object = (*env)->FindClass(env, "java/lang/Object");
	jmethodID to_string = (*env)->GetMethodID(env, object, "toString", "()Ljava/lang/String;");
	jstring s = (*env)->CallObjectMethod(env, t, to_string);
	const char *c = (*env)->GetStringUTFChars(env, s, NULL);
	char *out = strdup(c);
	(*env)->ReleaseStringUTFChars(env, s, c);
	(*env)->DeleteLocalRef(env, s);
	(*env)->DeleteLocalRef(env, object);
	(*env)->DeleteLocalRef(env, t);
	return out;
}

static char *go_string(JNIEnv *env, jstring s) {
	if (!s) {
		return NULL;
	}
	const char *c = (*env)->GetStringUTFChars(env, s, NULL);
	char *out = strdup(c);
	(*env)->ReleaseStringUTFChars(env, s, c);
	(*env)->DeleteLocalRef(env, s);
	return out;
}

// load_bridge defines the Bridge class from the dex, once per process.
static char *load_bridge(uintptr_t envp, uintptr_t ctx, void *dex, int len) {
	JNIEnv *env = (JNIEnv *)envp;
	if (bridge_class) {
		return NULL;
	}
	jobject buffer = (*env)->NewDirectByteBuffer(env, dex, len);
	jclass ctx_class = (*env)->GetObjectClass(env, (jobject)ctx);
	jmethodID get_loader = (*env)->GetMethodID(env, ctx_class, "getClassLoader", "()Ljava/lang/ClassLoader;");
	jobject parent = (*env)->CallObjectMethod(env, (jobject)ctx, get_loader);
	jclass dex_loader = (*env)->FindClass(env, "dalvik/system/InMemoryDexClassLoader");
	if (!dex_loader) {
		free(take_exception(env));
		return strdup("extracting pages needs Android 8 or newer");
	}
	jmethodID init = (*env)->GetMethodID(env, dex_loader, "<init>", "(Ljava/nio/ByteBuffer;Ljava/lang/ClassLoader;)V");
	jobject loader = (*env)->NewObject(env, dex_loader, init, buffer, parent);
	char *err = take_exception(env);
	if (err) {
		return err;
	}
	jclass class_loader = (*env)->FindClass(env, "java/lang/ClassLoader");
	jmethodID load_class = (*env)->GetMethodID(env, class_loader, "loadClass", "(Ljava/lang/String;)Ljava/lang/Class;");
	jstring name = (*env)->NewStringUTF(env, "io.wailscast.webextract.Bridge");
	jobject cls = (*env)->CallObjectMethod(env, loader, load_class, name);
	if ((err = take_exception(env))) {
		return err;
	}
	bridge_class = (*env)->NewGlobalRef(env, cls);
	return NULL;
}

static int bridge_supported(uintptr_t envp) {
	JNIEnv *env = (JNIEnv *)envp;
	jmethodID m = (*env)->GetStaticMethodID(env, bridge_class, "supported", "()Z");
	jboolean ok = (*env)->CallStaticBooleanMethod(env, bridge_class, m);
	free(take_exception(env));
	return ok;
}

static char *bridge_user_agent(uintptr_t envp, uintptr_t ctx) {
	JNIEnv *env = (JNIEnv *)envp;
	jmethodID m = (*env)->GetStaticMethodID(env, bridge_class, "userAgent", "(Landroid/app/Activity;)Ljava/lang/String;");
	jstring s = (*env)->CallStaticObjectMethod(env, bridge_class, m, (jobject)ctx);
	free(take_exception(env));
	return go_string(env, s);
}

// bridge_open creates a Bridge and opens the page; *out is a global ref.
static char *bridge_open(uintptr_t envp, uintptr_t ctx, const char *url, const char *script,
		const char **block, int nblock, uintptr_t *out) {
	JNIEnv *env = (JNIEnv *)envp;
	jmethodID init = (*env)->GetMethodID(env, bridge_class, "<init>", "(Landroid/app/Activity;)V");
	jobject bridge = (*env)->NewObject(env, bridge_class, init, (jobject)ctx);
	char *err = take_exception(env);
	if (err) {
		return err;
	}
	jclass string_class = (*env)->FindClass(env, "java/lang/String");
	jobjectArray blocks = (*env)->NewObjectArray(env, nblock, string_class, NULL);
	for (int i = 0; i < nblock; i++) {
		jstring b = (*env)->NewStringUTF(env, block[i]);
		(*env)->SetObjectArrayElement(env, blocks, i, b);
		(*env)->DeleteLocalRef(env, b);
	}
	jmethodID open = (*env)->GetMethodID(env, bridge_class, "open", "(Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;)V");
	jstring jurl = (*env)->NewStringUTF(env, url);
	jstring jscript = (*env)->NewStringUTF(env, script);
	(*env)->CallVoidMethod(env, bridge, open, jurl, jscript, blocks);
	if ((err = take_exception(env))) {
		return err;
	}
	*out = (uintptr_t)(*env)->NewGlobalRef(env, bridge);
	(*env)->DeleteLocalRef(env, bridge);
	(*env)->DeleteLocalRef(env, blocks);
	(*env)->DeleteLocalRef(env, jurl);
	(*env)->DeleteLocalRef(env, jscript);
	return NULL;
}

// bridge_next is the next message, or NULL after timeout_ms.
static char *bridge_next(uintptr_t envp, uintptr_t bridge, long timeout_ms) {
	JNIEnv *env = (JNIEnv *)envp;
	jmethodID m = (*env)->GetMethodID(env, bridge_class, "next", "(J)Ljava/lang/String;");
	jstring s = (*env)->CallObjectMethod(env, (jobject)bridge, m, (jlong)timeout_ms);
	free(take_exception(env));
	return go_string(env, s);
}

static char *bridge_cookies(uintptr_t envp, uintptr_t bridge, const char *url) {
	JNIEnv *env = (JNIEnv *)envp;
	jmethodID m = (*env)->GetMethodID(env, bridge_class, "cookies", "(Ljava/lang/String;)Ljava/lang/String;");
	jstring jurl = (*env)->NewStringUTF(env, url);
	jstring s = (*env)->CallObjectMethod(env, (jobject)bridge, m, jurl);
	(*env)->DeleteLocalRef(env, jurl);
	free(take_exception(env));
	return go_string(env, s);
}

static void bridge_close(uintptr_t envp, uintptr_t bridge) {
	JNIEnv *env = (JNIEnv *)envp;
	jmethodID m = (*env)->GetMethodID(env, bridge_class, "close", "()V");
	(*env)->CallVoidMethod(env, (jobject)bridge, m);
	free(take_exception(env));
	(*env)->DeleteGlobalRef(env, (jobject)bridge);
}
*/
import "C"

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unsafe"
)

// bridge.dex is built by cmd/androiddex (go generate ./pkg/webextract).
//
//go:embed bridge.dex
var bridgeDex []byte

// dexCopy keeps the dex in C memory for the process: the class loader may
// read it after loading.
var dexCopy = sync.OnceValue(func() unsafe.Pointer { return C.CBytes(bridgeDex) })

// JVMRunner runs fn on a thread attached to the app's JVM, with ctx the
// Activity (Fyne's driver.RunNative provides one).
type JVMRunner func(fn func(vm, env, ctx uintptr) error) error

// Android extracts in a full-screen WebView over the app; the page is always
// shown, so someone can tap play.
type Android struct {
	Run JVMRunner
}

func cError(err *C.char) error {
	if err == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(err))
	return errors.New(C.GoString(err))
}

func takeString(s *C.char) (string, bool) {
	if s == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(s))
	return C.GoString(s), true
}

// Extract shows pageURL until it plays an HLS stream, ctx ends or someone
// closes it.
func (a Android) Extract(ctx context.Context, pageURL string, rules Rules, opts Options) (*Result, error) {
	js, err := script(rules)
	if err != nil {
		return nil, err
	}
	var bridge C.uintptr_t
	var userAgent string
	err = a.Run(func(_, env, activity uintptr) error {
		dex := dexCopy()
		if err := cError(C.load_bridge(C.uintptr_t(env), C.uintptr_t(activity), dex, C.int(len(bridgeDex)))); err != nil {
			return fmt.Errorf("loading the WebView bridge: %w", err)
		}
		if C.bridge_supported(C.uintptr_t(env)) == 0 {
			return errors.New("this phone's WebView is too old to extract pages; update Android System WebView")
		}
		userAgent, _ = takeString(C.bridge_user_agent(C.uintptr_t(env), C.uintptr_t(activity)))
		curl, cjs := C.CString(pageURL), C.CString(js)
		defer C.free(unsafe.Pointer(curl))
		defer C.free(unsafe.Pointer(cjs))
		block := make([]*C.char, len(rules.Block)+1)
		for i, b := range rules.Block {
			block[i] = C.CString(b)
			defer C.free(unsafe.Pointer(block[i]))
		}
		return cError(C.bridge_open(C.uintptr_t(env), C.uintptr_t(activity), curl, cjs,
			(**C.char)(unsafe.Pointer(&block[0])), C.int(len(rules.Block)), &bridge))
	})
	if err != nil {
		return nil, err
	}
	defer a.Run(func(_, env, _ uintptr) error {
		C.bridge_close(C.uintptr_t(env), bridge)
		return nil
	})

	listenCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	messages := make(chan Message, 256)
	closed := false
	var reading sync.WaitGroup
	reading.Add(1)
	go func() {
		defer reading.Done()
		_ = a.Run(func(_, env, _ uintptr) error {
			for listenCtx.Err() == nil {
				text, ok := takeString(C.bridge_next(C.uintptr_t(env), bridge, 250))
				if !ok {
					continue
				}
				var m Message
				if json.Unmarshal([]byte(text), &m) != nil {
					continue
				}
				if m.Type == "closed" {
					closed = true
					cancel()
					return nil
				}
				select {
				case messages <- m:
				default:
				}
			}
			return nil
		})
	}()
	c, err := listen(listenCtx, pageURL, messages, opts)
	cancel()
	reading.Wait()
	if err != nil {
		if closed {
			return nil, errors.New("closed before the page played a stream")
		}
		return nil, err
	}
	streamURL := c.pick().URL
	var cookieHeader string
	_ = a.Run(func(_, env, _ uintptr) error {
		curl := C.CString(streamURL)
		defer C.free(unsafe.Pointer(curl))
		cookieHeader, _ = takeString(C.bridge_cookies(C.uintptr_t(env), bridge, curl))
		return nil
	})
	cookies := map[string]string{}
	for _, part := range strings.Split(cookieHeader, ";") {
		if name, value, ok := strings.Cut(strings.TrimSpace(part), "="); ok {
			cookies[name] = value
		}
	}
	return c.result(userAgent, cookies)
}
