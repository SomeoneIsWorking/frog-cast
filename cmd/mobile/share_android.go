//go:build android

package main

/*
#include <jni.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

// take_shared_text returns the text of a share intent the activity was
// started with, and replaces the intent so it is taken once; NULL if none.
static char *take_shared_text(uintptr_t envp, uintptr_t ctx) {
	JNIEnv *env = (JNIEnv *)envp;
	jobject activity = (jobject)ctx;
	jclass activity_class = (*env)->GetObjectClass(env, activity);
	jmethodID get_intent = (*env)->GetMethodID(env, activity_class, "getIntent", "()Landroid/content/Intent;");
	jobject intent = (*env)->CallObjectMethod(env, activity, get_intent);
	if (!intent) {
		return NULL;
	}
	jclass intent_class = (*env)->FindClass(env, "android/content/Intent");
	jmethodID get_action = (*env)->GetMethodID(env, intent_class, "getAction", "()Ljava/lang/String;");
	jstring action = (*env)->CallObjectMethod(env, intent, get_action);
	if (!action) {
		return NULL;
	}
	const char *a = (*env)->GetStringUTFChars(env, action, NULL);
	int send = strcmp(a, "android.intent.action.SEND") == 0;
	(*env)->ReleaseStringUTFChars(env, action, a);
	if (!send) {
		return NULL;
	}
	jmethodID get_extra = (*env)->GetMethodID(env, intent_class, "getStringExtra", "(Ljava/lang/String;)Ljava/lang/String;");
	jstring key = (*env)->NewStringUTF(env, "android.intent.extra.TEXT");
	jstring text = (*env)->CallObjectMethod(env, intent, get_extra, key);
	jmethodID init = (*env)->GetMethodID(env, intent_class, "<init>", "()V");
	jmethodID set_intent = (*env)->GetMethodID(env, activity_class, "setIntent", "(Landroid/content/Intent;)V");
	(*env)->CallVoidMethod(env, activity, set_intent, (*env)->NewObject(env, intent_class, init));
	if (!text) {
		return NULL;
	}
	const char *t = (*env)->GetStringUTFChars(env, text, NULL);
	char *out = strdup(t);
	(*env)->ReleaseStringUTFChars(env, text, t);
	return out;
}
*/
import "C"

import (
	"unsafe"

	"fyne.io/fyne/v2/driver"
)

// sharedText is the text shared to the app (from a browser's Share), once.
func sharedText() string {
	var text string
	_ = driver.RunNative(func(c any) error {
		ac := c.(*driver.AndroidContext)
		if s := C.take_shared_text(C.uintptr_t(ac.Env), C.uintptr_t(ac.Ctx)); s != nil {
			text = C.GoString(s)
			C.free(unsafe.Pointer(s))
		}
		return nil
	})
	return text
}
