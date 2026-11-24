# Desktop window is blank on Fedora KDE

Observed: on Fedora (KDE Plasma, Wayland, AMD Navi 22) the desktop app's window
opens with the KWin title bar and an empty #202326 client area. The backend and
the frontend's JavaScript run: the page calls `DiscoverDevices` right after
startup and the remote API answers.

Expected: the Vue frontend drawn in the window.

Reproduce: `wails build -tags webkit2_41`, run `build/bin/frog-cast` on the
desktop session, screenshot the window (`spectacle -b -n -a`).

Known so far:
- The Wails 2.11 build from 2026-11-18 looks the same, so the 2.17.1 bump did not cause it.
- `WEBKIT_DISABLE_DMABUF_RENDERER=1`, `WEBKIT_DISABLE_COMPOSITING_MODE=1` and
  `GDK_BACKEND=x11` do not change it.
- #202326 is not a colour the frontend uses; the web view does not seem to paint.
- Not yet checked by eye on the screen; a screenshot artefact is not ruled out.

The headless service (`frog-cast serve`) does not use the window and is unaffected.
