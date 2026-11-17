// Command androiddex builds pkg/webextract/bridge.dex, the Java side of the
// Android extraction host: Bridge.java plus androidx.webkit, which the app
// loads at runtime since Fyne's packager can't add Java classes to an APK.
// Run it (go generate ./pkg/webextract) before packaging the Android remote.
// It needs a JDK and ANDROID_HOME with build-tools and a platform.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

const (
	webkitURL    = "https://dl.google.com/android/maven2/androidx/webkit/webkit/1.18.0/webkit-1.18.0.aar"
	webkitSHA256 = "f15a0bd7d51f4bb8dffdd3a2cf5829f9a7de9ad4ba2682fa85d619032050581f"
	minAPI       = "26" // InMemoryDexClassLoader
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "androiddex:", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	sdk := os.Getenv("ANDROID_HOME")
	if sdk == "" {
		return fmt.Errorf("set ANDROID_HOME to the Android SDK")
	}
	androidJar, err := newest(filepath.Join(sdk, "platforms", "*", "android.jar"))
	if err != nil {
		return err
	}
	d8, err := newest(filepath.Join(sdk, "build-tools", "*", "d8"))
	if err != nil {
		return err
	}
	work := filepath.Join(root, "build", "androiddex")
	classes := filepath.Join(work, "classes")
	if err := os.RemoveAll(classes); err != nil {
		return err
	}
	if err := os.MkdirAll(classes, 0o755); err != nil {
		return err
	}
	webkitJar := filepath.Join(work, "webkit-classes.jar")
	if err := fetchClassesJar(filepath.Join(work, "webkit.aar"), webkitJar); err != nil {
		return err
	}
	src := filepath.Join(root, "pkg", "webextract", "android", "Bridge.java")
	if err := command("javac", "--release", "17", "-nowarn", "-classpath", androidJar+string(os.PathListSeparator)+webkitJar,
		"-d", classes, src); err != nil {
		return err
	}
	bridgeClasses, err := filepath.Glob(filepath.Join(classes, "io", "frogcast", "webextract", "*.class"))
	if err != nil {
		return err
	}
	args := append([]string{"--release", "--min-api", minAPI, "--lib", androidJar, "--output", work}, bridgeClasses...)
	if err := command(d8, append(args, webkitJar)...); err != nil {
		return err
	}
	return os.Rename(filepath.Join(work, "classes.dex"), filepath.Join(root, "pkg", "webextract", "bridge.dex"))
}

func repoRoot() (string, error) {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return "", err
	}
	return filepath.Dir(string(out[:len(out)-1])), nil
}

// newest is the last match of pattern in version order.
func newest(pattern string) (string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", fmt.Errorf("nothing matches %s", pattern)
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

func command(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(name), err)
	}
	return nil
}

// fetchClassesJar downloads the androidx.webkit AAR once, checks it, and
// extracts its classes.jar.
func fetchClassesJar(aar, jar string) error {
	if !matches(aar) {
		resp, err := http.Get(webkitURL)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s: %s", webkitURL, resp.Status)
		}
		f, err := os.Create(aar)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, resp.Body); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		if !matches(aar) {
			os.Remove(aar)
			return fmt.Errorf("%s: checksum mismatch", webkitURL)
		}
	}
	r, err := zip.OpenReader(aar)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Name != "classes.jar" {
			continue
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(jar)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}
	return fmt.Errorf("%s has no classes.jar", aar)
}

func matches(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) == webkitSHA256
}
