"""Build frog-cast and install it as a systemd user service (Linux).

Builds the frontend and the binary, installs the binary to ~/.local/bin/frog-cast
and packaging/systemd/frog-cast.service to ~/.config/systemd/user, then enables
and starts (or restarts) the unit. A running instance that is casting is not
restarted unless --force-restart is given.
"""

import argparse
import json
import os
import shutil
import subprocess
import sys
import urllib.request
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
UNIT_NAME = "frog-cast.service"
UNIT_SOURCE = REPO / "packaging" / "systemd" / UNIT_NAME
BUILD_OUTPUT = REPO / "build" / "bin" / "service" / "frog-cast"
GO_TAGS = "desktop,production,webkit2_41"
PKG_CONFIG_MODULES = ["webkit2gtk-4.1", "gtk+-3.0", "libsoup-3.0"]
NATIVE_INSTALL = {
    "Fedora": "sudo dnf install golang nodejs-npm webkit2gtk4.1-devel gtk3-devel libsoup3-devel",
    "Debian/Ubuntu": "sudo apt install golang npm libwebkit2gtk-4.1-dev libgtk-3-dev libsoup-3.0-dev",
}


def refuse(message: str) -> None:
    lines = [f"error: {message}", "Install the build dependencies:"]
    lines += [f"  {distro}: {command}" for distro, command in NATIVE_INSTALL.items()]
    sys.exit("\n".join(lines))


def find_npm() -> str:
    npm = shutil.which("npm")
    if npm:
        return npm
    # nvm installs are not on a non-interactive PATH.
    nvm_versions = Path(os.environ.get("NVM_DIR", Path.home() / ".nvm")) / "versions" / "node"
    candidates = sorted(nvm_versions.glob("*/bin/npm"))
    if candidates:
        return str(candidates[-1])
    refuse("npm not found")
    return ""


def check_toolchain() -> None:
    if not shutil.which("go"):
        refuse("go not found")
    if not shutil.which("pkg-config"):
        refuse("pkg-config not found")
    missing = [m for m in PKG_CONFIG_MODULES if subprocess.run(["pkg-config", "--exists", m]).returncode != 0]
    if missing:
        refuse("missing development packages: " + ", ".join(missing))


def run(command: list[str], cwd: Path = REPO, env: dict[str, str] | None = None) -> None:
    print("==>", " ".join(command), flush=True)
    subprocess.run(command, cwd=cwd, env=env, check=True)


def build(skip_frontend: bool) -> None:
    check_toolchain()
    if not skip_frontend:
        npm = find_npm()
        env = dict(os.environ, PATH=f"{Path(npm).parent}{os.pathsep}{os.environ.get('PATH', '')}")
        frontend = REPO / "frontend"
        if not (frontend / "node_modules").is_dir():
            run([npm, "install"], cwd=frontend, env=env)
        run([npm, "run", "build"], cwd=frontend, env=env)
    BUILD_OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    run(["go", "build", "-tags", GO_TAGS, "-ldflags", "-w -s", "-o", str(BUILD_OUTPUT), "."])


def install_file(source: Path, dest: Path, mode: int) -> None:
    """Replace dest atomically, so a running binary keeps its old inode."""
    dest.parent.mkdir(parents=True, exist_ok=True)
    staging = dest.with_name(dest.name + ".new")
    shutil.copyfile(source, staging)
    staging.chmod(mode)
    staging.replace(dest)
    print(f"==> installed {dest}")


def systemctl(*args: str, check: bool = True) -> subprocess.CompletedProcess:
    return subprocess.run(["systemctl", "--user", *args], check=check, capture_output=not check, text=True)


def casting_status(port: int) -> str:
    """The running instance's playback status, or "" when it does not answer."""
    try:
        with urllib.request.urlopen(f"http://127.0.0.1:{port}/state", timeout=3) as response:
            return json.load(response).get("status", "")
    except (OSError, ValueError):
        return ""


def start(port: int, force_restart: bool) -> None:
    systemctl("daemon-reload")
    systemctl("enable", UNIT_NAME)
    if systemctl("is-active", "--quiet", UNIT_NAME, check=False).returncode != 0:
        systemctl("start", UNIT_NAME)
        print(f"==> started {UNIT_NAME}")
        return
    status = casting_status(port)
    if status in ("PLAYING", "PAUSED", "LOADING") and not force_restart:
        print(f"==> {UNIT_NAME} is casting ({status}): not restarting it; the new binary runs after "
              f"`systemctl --user restart {UNIT_NAME}` (or rerun with --force-restart)")
        return
    systemctl("restart", UNIT_NAME)
    print(f"==> restarted {UNIT_NAME}")


def check_linger() -> None:
    user = os.environ.get("USER") or Path.home().name
    result = subprocess.run(["loginctl", "show-user", user, "-p", "Linger", "--value"],
                            capture_output=True, text=True)
    if result.stdout.strip() != "yes":
        print(f"warning: lingering is off for {user}: the service stops at logout and starts only at login.\n"
              f"  To run it from boot without a login: loginctl enable-linger {user}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--skip-build", action="store_true", help="install the last build without rebuilding")
    parser.add_argument("--skip-frontend", action="store_true", help="reuse the built frontend/dist")
    parser.add_argument("--port", type=int, default=9999,
                        help="remote API port, to ask a running instance whether it is casting (default 9999)")
    parser.add_argument("--force-restart", action="store_true", help="restart the service even while it casts")
    args = parser.parse_args()

    if sys.platform != "linux":
        sys.exit("error: the systemd user service is Linux-only")
    if not args.skip_build:
        build(args.skip_frontend)
    if not BUILD_OUTPUT.is_file():
        sys.exit(f"error: no build at {BUILD_OUTPUT}; run without --skip-build")
    install_file(BUILD_OUTPUT, Path.home() / ".local" / "bin" / "frog-cast", 0o755)
    install_file(UNIT_SOURCE, Path.home() / ".config" / "systemd" / "user" / UNIT_NAME, 0o644)
    start(args.port, args.force_restart)
    check_linger()


if __name__ == "__main__":
    main()
