package io.wailscast.webextract;

import android.app.Activity;
import android.app.Dialog;
import android.view.ViewGroup;
import android.webkit.CookieManager;
import android.webkit.JavascriptInterface;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import androidx.webkit.WebViewCompat;
import androidx.webkit.WebViewFeature;
import java.io.ByteArrayInputStream;
import java.util.Collections;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.TimeUnit;

// One extraction: the page in a full-screen WebView dialog, capture.js in
// every frame, and its messages queued for Go to take.
public final class Bridge {
    private final Activity activity;
    private final LinkedBlockingQueue<String> messages = new LinkedBlockingQueue<>();
    private Dialog dialog;
    private WebView view;

    public Bridge(Activity activity) {
        this.activity = activity;
    }

    public static boolean supported() {
        return WebViewFeature.isFeatureSupported(WebViewFeature.DOCUMENT_START_SCRIPT);
    }

    public static String userAgent(Activity activity) {
        return WebSettings.getDefaultUserAgent(activity);
    }

    public void open(String url, String script, String[] block) {
        activity.runOnUiThread(() -> {
            view = new WebView(activity);
            WebSettings settings = view.getSettings();
            settings.setJavaScriptEnabled(true);
            settings.setDomStorageEnabled(true);
            settings.setMediaPlaybackRequiresUserGesture(false);
            CookieManager.getInstance().setAcceptThirdPartyCookies(view, true);
            view.addJavascriptInterface(this, "CastBridge");
            WebViewCompat.addDocumentStartJavaScript(view, script, Collections.singleton("*"));
            view.setWebViewClient(new WebViewClient() {
                @Override
                public WebResourceResponse shouldInterceptRequest(WebView v, WebResourceRequest request) {
                    String target = request.getUrl().toString();
                    for (String b : block) {
                        if (target.contains(b)) {
                            return new WebResourceResponse("text/plain", "utf-8", new ByteArrayInputStream(new byte[0]));
                        }
                    }
                    return null;
                }
            });
            dialog = new Dialog(activity, android.R.style.Theme_DeviceDefault_NoActionBar);
            dialog.setContentView(view, new ViewGroup.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
            dialog.setOnDismissListener(d -> messages.offer("{\"type\":\"closed\"}"));
            dialog.show();
            view.loadUrl(url);
        });
    }

    // next is the next message, or null after timeoutMs.
    public String next(long timeoutMs) throws InterruptedException {
        return messages.poll(timeoutMs, TimeUnit.MILLISECONDS);
    }

    public String cookies(String url) {
        return CookieManager.getInstance().getCookie(url);
    }

    public void close() {
        activity.runOnUiThread(() -> {
            if (dialog != null) {
                dialog.setOnDismissListener(null);
                dialog.dismiss();
            }
            if (view != null) {
                view.destroy();
            }
        });
    }

    @JavascriptInterface
    public void postMessage(String message) {
        messages.offer(message);
    }
}
