package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"frog-cast/pkg/castapi"
	"frog-cast/pkg/folders"
	"frog-cast/pkg/remote"
)

func TestRunCommandHelpAndDesktopFallthrough(t *testing.T) {
	var out bytes.Buffer
	handled, err := runCommand([]string{"--help"}, &out)
	if !handled || err != nil || !strings.Contains(out.String(), "frog-cast serve") {
		t.Fatalf("--help: handled=%v err=%v out=%q", handled, err, out.String())
	}
	out.Reset()
	handled, err = runCommand([]string{"serve", "--help"}, &out)
	if !handled || err != nil || !strings.Contains(out.String(), "frog-cast serve") {
		t.Fatalf("serve --help: handled=%v err=%v out=%q", handled, err, out.String())
	}
	for _, args := range [][]string{nil, {"-somethingWailsPasses"}} {
		if handled, _ := runCommand(args, &out); handled {
			t.Fatalf("%q did not open the desktop app", args)
		}
	}
}

// freePort returns a port nothing listens on.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func headlessApp(t *testing.T, mediaPort, apiPort int) *App {
	t.Helper()
	a, _ := newTestApp(t)
	a.mediaServer = NewServer(mediaPort)
	a.port = mediaPort
	a.RemoteManager = remote.NewManager(true)
	a.httpServer = NewHTTPServer(a)
	settings := *a.settingsStore.Get()
	settings.RemoteAPIEnabled = false
	settings.RemoteAPIPort = apiPort
	if err := a.settingsStore.Update(settings); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestServeHeadlessRunsTheRemoteAPIEvenWhenDisabled(t *testing.T) {
	apiPort := freePort(t)
	a := headlessApp(t, freePort(t), apiPort)
	if err := a.serveHeadless(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer a.shutdown(context.Background())
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/state", apiPort))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/state = %d", resp.StatusCode)
	}
	if _, err := a.OpenFileDialog("", nil); !errors.Is(err, errNoWindow) {
		t.Fatalf("file dialog headless: %v", err)
	}
	if _, err := a.OpenLibraryFolderDialog(); !errors.Is(err, errNoWindow) {
		t.Fatalf("folder dialog headless: %v", err)
	}
}

func TestServeHeadlessFailsWhenTheMediaPortIsTaken(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	a := headlessApp(t, ln.Addr().(*net.TCPAddr).Port, freePort(t))
	defer a.shutdown(context.Background())
	if err := a.serveHeadless(context.Background()); err == nil || !strings.Contains(err.Error(), "media server") {
		t.Fatalf("err = %v, want the media server's bind error", err)
	}
}

func TestHeadlessRefusesAScriptedPageItCannotExtract(t *testing.T) {
	dir := filepath.Join(folders.GetConfig(), "scripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "headless-test.lua")
	if err := os.WriteFile(script, []byte(`match_patterns = {"scripted.example.test/*"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(script)

	a, _ := newTestApp(t)
	a.RemoteManager = remote.NewManager(true)
	// No GTK loop runs in tests, as in a headless instance.
	err := a.readyPage("https://scripted.example.test/film/", 0, 0, false)
	if !errors.Is(err, castapi.ErrNeedsExtraction) {
		t.Fatalf("err = %v, want needs-extraction", err)
	}
}

func TestLocalIPForUsesTheRouteToTheDevice(t *testing.T) {
	var dd *DeviceDiscovery
	if got := dd.LocalIPFor("127.0.0.1"); got != "127.0.0.1" {
		t.Fatalf("toward loopback = %s", got)
	}
	if got := dd.LocalIPFor(""); got == "" {
		t.Fatal("no fallback address")
	}
}
