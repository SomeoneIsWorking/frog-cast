# Desktop window is blank on Fedora KDE

Observed: on Fedora (KDE Plasma 6.8.3, Wayland, WebKitGTK 2.54.2) the desktop
app's window opens with the KWin title bar and an empty #202326 client area,
while the backend and the frontend run (App.vue's onMounted calls
`DiscoverDevices`).

Expected: the Vue frontend drawn in the window.

Cause (suspected, unconfirmed): the only output, HDMI-A-1, was in DPMS off
(`kscreen-doctor --dpms show`). KWin sends no frame callbacks to an output that
is off, and WebKitGTK on Wayland paints only on them, so every WebKit window
keeps its first blank frame. The stock `/usr/libexec/webkit2gtk-4.1/MiniBrowser`
on a solid red page captures the same #202326, and a full-screen capture is
black. The Wails 2.11 and 2.17.1 builds behave the same, and
`WEBKIT_DISABLE_DMABUF_RENDERER=1`, `WEBKIT_DISABLE_COMPOSITING_MODE=1` and
`GDK_BACKEND=x11` change nothing.

To confirm: with the display awake, run `build/bin/frog-cast` on the desktop
session and screenshot the window (`spectacle -b -n -a`). If the UI shows, this
is not an app defect and the issue closes.
