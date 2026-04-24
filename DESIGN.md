# Design Document: `setup.py` — Python 3 Replacement for Shell Scripts

## 1. File Structure

### Single new file

```
setup.py          # ~900-1100 lines, replaces setup + diagnose + all .sh libs + subcmd scripts
```

### Bash scripts to DELETE (replaced by `setup.py`)

| File                                     | Lines | Role                                |
| ---------------------------------------- | ----- | ----------------------------------- |
| `setup`                                  | 130   | Main entry point                    |
| `diagnose`                               | 112   | Diagnostic collector                |
| `sdata/lib/environment-variables.sh`     | 30    | XDG dirs, paths, styling            |
| `sdata/lib/functions.sh`                 | 471   | Utility functions                   |
| `sdata/lib/package-installers.sh`        | 78    | venv, font, package installers      |
| `sdata/lib/dist-determine.sh`            | 116   | Distro detection                    |
| `sdata/dist-arch/install-deps.sh`        | 65    | Arch package install                |
| `sdata/dist-arch/uninstall-deps.sh`      | 14    | Arch package uninstall              |
| `sdata/dist-fedora/install-deps.sh`      | 138   | Fedora package install              |
| `sdata/dist-fedora/uninstall-deps.sh`    | 11    | Fedora package uninstall            |
| `sdata/subcmd-install/options.sh`        | ~80   | Install CLI args                    |
| `sdata/subcmd-install/0.greeting.sh`     | ~40   | Greeting/prompt                     |
| `sdata/subcmd-install/1.deps-router.sh`  | ~60   | Deps routing + outdate detect       |
| `sdata/subcmd-install/2.setups.sh`       | ~60   | Setups (groups, systemd, gsettings) |
| `sdata/subcmd-install/3.files.sh`        | ~90   | File copy orchestrator              |
| `sdata/subcmd-install/3.files-legacy.sh` | ~80   | Legacy file copy logic              |
| `sdata/subcmd-install/3.files-exp.sh`    | ~200  | Experimental yaml file copy         |
| `sdata/subcmd-uninstall/0.run.sh`        | ~100  | Uninstall logic                     |
| `sdata/subcmd-uninstall/options.sh`      | ~20   | Uninstall CLI args                  |
| `sdata/subcmd-checkdeps/0.run.sh`        | ~25   | AUR dep checker                     |
| `sdata/subcmd-checkdeps/options.sh`      | ~15   | Checkdeps CLI args                  |
| `sdata/subcmd-resetfirstrun/0.run.sh`    | ~5    | Reset firstrun marker               |
| `sdata/subcmd-resetfirstrun/options.sh`  | ~15   | Reset CLI args                      |

### Bash scripts to KEEP (not replaced)

| File                               | Why kept                                        |
| ---------------------------------- | ----------------------------------------------- |
| `sdata/subcmd-exp-update/0.run.sh` | 1200 lines, complex git logic, AI-generated     |
| `sdata/subcmd-exp-merge/0.run.sh`  | 340 lines, complex git rebase logic             |
| `sdata/subcmd-virtmon/0.run.sh`    | Dev-only, tightly coupled to hyprctl/wayvnc CLI |

These will be called by `setup.py` as subprocesses (sourced from their existing bash infrastructure).

### Data files to KEEP untouched

- `sdata/dist-arch/packages.conf`
- `sdata/dist-arch/illogical-impulse-microtex-git/PKGBUILD`
- `sdata/dist-fedora/feddeps.toml`
- `sdata/dist-fedora/SPECS/`
- `sdata/dist-fedora/user_data.yaml`
- `sdata/uv/requirements.txt`
- `sdata/subcmd-install/3.files-exp.yaml`
- `dots/` entire directory tree
- `dots-extra/fontsets/`
- `os-release` (custom override)

### Backward-compat wrapper

Replace the existing `setup` bash script with a thin wrapper:

```bash
#!/usr/bin/env bash
cd "$(dirname "$0")"
exec python3 setup.py "$@"
```

This preserves the `./setup install` UX.

---

## 2. Class & Function Signatures

### Module-level constants

```python
VERSION = "2.0.0"
REPO_ROOT = Path(__file__).resolve().parent

# XDG paths
XDG_BIN_HOME    = Path(os.environ.get("XDG_BIN_HOME",    Path.home() / ".local/bin"))
XDG_CACHE_HOME  = Path(os.environ.get("XDG_CACHE_HOME",   Path.home() / ".cache"))
XDG_CONFIG_HOME = Path(os.environ.get("XDG_CONFIG_HOME",  Path.home() / ".config"))
XDG_DATA_HOME   = Path(os.environ.get("XDG_DATA_HOME",   Path.home() / ".local/share"))
XDG_STATE_HOME  = Path(os.environ.get("XDG_STATE_HOME",  Path.home() / ".local/state"))

ILLOGICAL_IMPULSE_VIRTUAL_ENV = Path(os.environ.get(
    "ILLOGICAL_IMPULSE_VIRTUAL_ENV",
    XDG_STATE_HOME / "quickshell" / ".venv"
))

BACKUP_DIR        = Path(os.environ.get("BACKUP_DIR", Path.home() / "ii-original-dots-backup"))
DOTS_CORE_CONFDIR = XDG_CONFIG_HOME / "illogical-impulse"
FIRSTRUN_FILE     = DOTS_CORE_CONFDIR / "installed_true"
INSTALLED_LISTFILE = DOTS_CORE_CONFDIR / "installed_listfile"

# ANSI styling
class _C:
    RED    = "\033[31m"; GREEN  = "\033[32m"; YELLOW = "\033[33m"
    BLUE   = "\033[34m"; PURPLE = "\033[35m"; CYAN   = "\033[36m"
    BOLD   = "\033[1m";  FAINT  = "\033[2m";  SLANT  = "\033[3m"
    UNDER  = "\033[4m";  INVERT = "\033[7m";  RST    = "\033[0m"
```

### `class Shell`

Wraps `subprocess` with interactive error recovery and sudo keepalive.

```python
class Shell:
    def __init__(self, repo_root: Path, interactive: bool = True):
        """
        interactive: when True, prompts user on command failure (mirrors bash v()/x()).
                     when False, auto-retries once then exits (mirrors --force mode).
        """

    def run(self, cmd: list[str], *, check: bool = True, sudo: bool = False,
            cwd: Path | None = None, env: dict | None = None) -> subprocess.CompletedProcess:
        """Execute a single command. On failure in interactive mode, offer retry/ignore/exit.
           On failure in non-interactive mode, exit immediately.
           Appends sudo if sudo=True.
        """

    def run_quiet(self, cmd: list[str], **kwargs) -> subprocess.CompletedProcess:
        """Like run() but suppresses stdout. Returns CompletedProcess."""

    @staticmethod
    def command_exists(name: str) -> bool:
        """shutil.which() wrapper."""

    @staticmethod
    def require_command(name: str) -> None:
        """Exit if command not found."""

    def sudo_init_keepalive(self) -> None:
        """Prompt for sudo once, spin a background thread that runs `sudo -v` every 55s."""

    def sudo_stop_keepalive(self) -> None:
        """Terminate the keepalive thread."""

    def pause(self, message: str = "Ctrl-C to abort, Enter to proceed") -> None:
        """Read a single Enter from tty, or skip in non-interactive mode."""

    def ask_yes_no(self, prompt: str, default: bool = True) -> bool:
        """Interactive y/n prompt. Returns default in non-interactive mode."""
```

### `class Distro`

```python
class Distro:
    def __init__(self, repo_root: Path):
        self.repo_root = repo_root
        self.os_distro_id: str = ""       # e.g. "arch", "cachyos", "fedora"
        self.os_distro_id_like: str = ""   # e.g. "arch"
        self.os_group_id: str = ""         # "arch" | "fedora" | "fallback"
        self.machine_arch: str = ""         # e.g. "x86_64"

    def detect(self) -> None:
        """Read /etc/os-release (or repo_root/os-release if it exists) and populate fields."""

    def print_info(self) -> None:
        """Print detected OS info + warnings for 'alike', 'unofficial', 'unsupported' distros."""

    def is_arch(self) -> bool:
        return self.os_group_id == "arch"

    def is_fedora(self) -> bool:
        return self.os_group_id == "fedora"

    def dist_dir(self) -> Path:
        """Return sdata/dist-{self.os_group_id}/"""
```

### `class PackageManager`

```python
class PackageManager:
    def __init__(self, distro: Distro, shell: Shell):
        self.distro = distro
        self.sh = shell

    def install_packages(self, packages: list[str]) -> None:
        """Dispatcher: arch → yay, fedora → dnf."""

    def remove_packages(self, packages: list[str]) -> None:
        """Arch → yay -Rns, fedora → dnf history undo (transaction-based)."""

    def _install_arch(self) -> None:
        """Full Arch/CachyOS install flow: system update, yay install if missing,
           packages from packages.conf, local PKGBUILD build."""

    def _uninstall_arch(self) -> None:
        """Remove packages from packages.conf + illogical-impulse-microtex-git."""

    def _install_fedora(self) -> None:
        """Full Fedora install flow: system upgrade, COPR repos, RPM builds,
           packages from feddeps.toml, versionlock."""

    def _uninstall_fedora(self) -> None:
        """Roll back DNF transactions recorded in user_data.yaml."""

    def install_cmds(self, cmds: list[str]) -> None:
        """Map command names to package names per distro, then install missing ones."""

    def ensure_cmds(self, cmds: list[str]) -> None:
        """Check which commands are missing, then install_cmds for those."""

    def install_python_packages(self) -> None:
        """Create uv venv at ILLOGICAL_IMPULSE_VIRTUAL_ENV and install requirements.txt."""

    def install_google_sans_flex(self) -> None:
        """Clone google-sans-flex font repo, rsync, fc-cache."""

    def install_font(self, name: str) -> None:
        """Install named font from cache (e.g., "Rubik", "Gabarito", "bibata")."""

    def read_packages_conf(self) -> list[str]:
        """Parse sdata/dist-arch/packages.conf → stripped package list."""

    def read_feddeps_toml(self) -> dict:
        """Parse sdata/dist-fedora/feddeps.toml using tomllib (3.11+) or fallback yaml parser."""
```

### `class FileSync`

```python
class FileSync:
    def __init__(self, shell: Shell, installed_listfile: Path, interactive: bool = True):
        self.sh = shell
        self.installed_listfile = installed_listfile
        self.interactive = interactive

    def install_file(self, src: Path, dst: Path) -> None:
        """cp -f src dst, record in installed_listfile."""

    def install_file__auto_backup(self, src: Path, dst: Path, firstrun: bool) -> None:
        """If dst exists and firstrun→mv dst dst.old, else cp to dst.new.
           If dst absent→cp directly. Records in installed_listfile."""

    def install_dir(self, src: Path, dst: Path) -> None:
        """rsync -a src/ dst/, record files in installed_listfile."""

    def install_dir__sync(self, src: Path, dst: Path) -> None:
        """rsync -a --delete src/ dst/, record files."""

    def install_dir__sync_exclude(self, src: Path, dst: Path, excludes: list[str]) -> None:
        """rsync -a --delete --exclude=... src/ dst/, record files."""

    def install_dir__ignore_existing(self, src: Path, dst: Path) -> None:
        """rsync -a --ignore-existing src/ dst/, record files."""

    def install_dir__skip_ifexist(self, src: Path, dst: Path) -> None:
        """If dst exists, skip; otherwise rsync -a."""

    def backup_clashing_targets(self, src_dir: Path, target_dir: Path,
                                 backup_dir: Path, ignored: list[str] = []) -> None:
        """Find clashing entries, rsync them to backup_dir."""

    def auto_backup_configs(self, skip: bool = False) -> None:
        """Interactive/automatic backup of .config and .local/share clashes."""

    def gen_firstrun(self) -> None:
        """Touch FIRSTRUN_FILE, append to installed_listfile."""

    @staticmethod
    def dedup_and_sort_listfile(path: Path) -> None:
        """Sort-unique the listfile in-place."""

    def install_exp_patterns(self, config_path: Path, fontset_dir_name: str = "",
                              preferences: dict | None = None) -> None:
        """Parse YAML config, evaluate conditions, call appropriate install_dir/install_file
           variant based on 'mode' field. Supports sync/soft/hard/hard-backup/soft-backup/
           skip/skip-if-exists."""
```

### `class Setup` (main orchestrator)

```python
class Setup:
    def __init__(self, args: argparse.Namespace):
        self.args = args
        self.repo_root = REPO_ROOT
        self.interactive = not args.force
        self.sh = Shell(self.repo_root, interactive=self.interactive)
        self.distro = Distro(self.repo_root)
        self.distro.detect()
        self.pkg = PackageManager(self.distro, self.sh)
        self.fsync = FileSync(self.sh, INSTALLED_LISTFILE, interactive=self.interactive)

    # --- Subcommands ---
    def cmd_install(self) -> None:
        """Full install: distro info → greeting → deps → setups → files → firstrun."""

    def cmd_install_deps(self) -> None:
        """Just the deps step."""

    def cmd_install_setups(self) -> None:
        """Just the setups step."""

    def cmd_install_files(self) -> None:
        """Just the files step."""

    def cmd_uninstall(self) -> None:
        """Undo install: remove files, undo groups, remove packages."""

    def cmd_diagnose(self) -> None:
        """Collect system info and optionally upload to pastebin."""

    def cmd_resetfirstrun(self) -> None:
        """Delete FIRSTRUN_FILE."""

    def cmd_checkdeps(self) -> None:
        """Check AUR/repo package existence from packages.conf."""

    # --- Internal helpers ---
    def _greeting(self) -> None:
        """Print welcome message, briefly describe steps, ask confirm-each-command preference."""

    def _install_deps(self) -> None:
        """Route to arch/fedora installer, run outdate detection for non-arch distros."""

    def _install_setups(self) -> None:
        """Python venv, user groups, systemd services, gsettings, kwriteconfig6."""

    def _install_files(self) -> None:
        """Choose legacy or yaml-based file copy, then google-sans-flex, firstrun, hyprctl reload."""

    def _install_files_legacy(self) -> None:
        """Mirrors 3.files-legacy.sh behavior."""

    def _outdate_detect(self, source_path: Path, target_path: Path) -> str:
        """Compare git timestamps, read outdate-detect-mode. Returns status string."""

    def _outdate_check(self) -> None:
        """Warn if dist-specific files are outdated (for non-arch distros)."""
```

---

## 3. CLI Argument Structure

### Top-level parser

```
python3 setup.py <subcommand> [OPTIONS]

Subcommands:
  install          Full install (deps → setups → files)
  install-deps     Just deps step
  install-setups   Just setups step
  install-files    Just files step
  uninstall        Remove installed files, undo groups, remove packages
  diagnose         Collect system info for bug reports
  resetfirstrun    Delete firstrun marker
  checkdeps        Check AUR package existence (dev-only)
  help             Show global help

  # Passthrough subcommands (call existing bash scripts):
  exp-update       Delegate to sdata/subcmd-exp-update/0.run.sh
  exp-merge        Delegate to sdata/subcmd-exp-merge/0.run.sh
  virtmon          Delegate to sdata/subcmd-virtmon/0.run.sh
```

### `install` subcommand flags

```
  -h, --help
  -f, --force                Non-interactive, auto-confirm everything
  -F, --firstrun             Act like first run (overwrite instead of .new)
  -c, --clean                Clean the build cache first
      --skip-allgreeting     Skip greeting
      --skip-alldeps         Skip deps step
      --skip-allsetups       Skip setups step
      --skip-allfiles        Skip files step
      --ignore-outdate       Ignore outdate checking for dist-*
  -s, --skip-sysupdate       Skip system package upgrade
      --skip-plasmaintg      Skip plasma-browser-integration
      --skip-backup          Skip backup of conflicting files
      --skip-quickshell      Skip Quickshell config
      --skip-hyprland        Skip Hyprland config
      --skip-hyprland-entry  Skip Hyprland entry config (hyprland.conf)
      --skip-fish            Skip Fish config
      --skip-fontconfig      Skip fontconfig
      --skip-miscconf        Skip misc .config dirs
      --core                 Alias for --skip-{plasmaintg,fish,miscconf,fontconfig}
      --exp-files            Use yaml-based config for file copy step
      --fontset <set>        Use a pre-defined fontset from dots-extra/fontsets/
```

### `install-deps`, `install-setups`, `install-files` flags

Same relevant subset of `install` flags (e.g., `install-deps` accepts `--force`, `--skip-sysupdate`, `--ignore-outdate`).

### `uninstall` flags

```
  -h, --help
  -f, --force       Non-interactive
```

### `diagnose` flags

```
  -h, --help
      --no-upload    Skip pastebin upload prompt
```

### `resetfirstrun` flags

```
  -h, --help
```

### `checkdeps` flags

```
  -h, --help
      --list-file <path>   Override default packages.conf path
```

### `exp-update`, `exp-merge`, `virtmon` — passthrough

All unknown arguments are passed through to the underlying bash script.

---

## 4. How Each Subcommand Works in Python

### 4.1 `cmd_install()`

```python
def cmd_install(self):
    self._prevent_sudo_or_root()
    self.distro.print_info()
    self.sh.pause()
    self.sh.sudo_init_keepalive()
    try:
        if not self.args.skip_allgreeting:
            self._greeting()
        if not self.args.skip_alldeps:
            self._install_deps()
        if not self.args.skip_allsetups:
            self._install_setups()
        if not self.args.skip_allfiles:
            self._install_files()
    finally:
        self.sh.sudo_stop_keepalive()
    self._print_post_install_message()
```

### 4.2 `_greeting()`

Print the welcome message (Quickshell notice, overview, tips). In interactive mode, prompt the user whether to confirm each command. Set `self.interactive` accordingly.

### 4.3 `_install_deps()`

```python
def _install_deps(self):
    # Outdate detection for non-arch
    if not self.distro.is_arch():
        self._outdate_check()

    # Dispatch to distro-specific installer
    if self.distro.is_arch():
        self.pkg._install_arch()
    elif self.distro.is_fedora():
        self.pkg._install_fedora()
    else:
        self.sh.log_warning("No installer for this distro. Install deps manually.")
```

**Arch installer details:**

1. `sudo pacman -Syu` (unless `--skip-sysupdate`)
2. Install `yay` if missing (clone, `makepkg`)
3. Read `sdata/dist-arch/packages.conf`, parse into list (strip comments/blanks)
4. `yay -S --needed --noconfirm <packages>`
5. Build and install local PKGBUILD: `sdata/dist-arch/illogical-impulse-microtex-git/`

**Fedora installer details:**

1. `sudo dnf upgrade --refresh -y` (unless `--skip-sysupdate`)
2. Remove versionlock on `quickshell-git`
3. Install `yq` for config parsing
4. Install `@development-tools fedora-packager`
5. Enable COPR repos from `feddeps.toml`
6. Build RPM specs from `sdata/dist-fedora/SPECS/`
7. Install package groups from `feddeps.toml`
8. Re-add versionlock on `quickshell-git`

### 4.4 `_install_setups()`

```python
def _install_setups(self):
    # Python venv
    self.pkg.install_python_packages()

    # User groups
    self._setup_user_groups()

    # Systemd services
    self._setup_systemd_services()

    # gsettings / kwriteconfig6
    self.sh.run(["gsettings", "set", "org.gnome.desktop.interface", "font-name",
                  "Google Sans Flex Medium 11 @opsz=11,wght=500"])
    self.sh.run(["gsettings", "set", "org.gnome.desktop.interface", "color-scheme", "prefer-dark"])
    self.sh.run(["kwriteconfig6", "--file", "kdeglobals", "--group", "KDE", "--key", "widgetStyle", "Darkly"])
```

### 4.5 `_install_files()`

```python
def _install_files(self):
    # Ensure XDG dirs exist
    for d in [XDG_BIN_HOME, XDG_CACHE_HOME, XDG_CONFIG_HOME, XDG_DATA_HOME]:
        d.mkdir(parents=True, exist_ok=True)

    # Determine firstrun state
    firstrun = getattr(self.args, 'firstrun', None)
    if firstrun is None:
        firstrun = not FIRSTRUN_FILE.exists()

    # Git submodule update
    self.sh.run(["git", "submodule", "update", "--init", "--recursive"], check=False)

    # Backup
    if not self.args.skip_backup:
        self.fsync.auto_backup_configs()

    # Choose file copy mode
    if getattr(self.args, 'exp_files', False):
        self.fsync.install_exp_patterns(
            self.repo_root / "sdata/subcmd-install/3.files-exp.yaml",
            fontset_dir_name=getattr(self.args, 'fontset', '') or '',
            preferences=self._collect_preferences(),
        )
    else:
        self._install_files_legacy(firstrun)

    # Google Sans Flex (skip on Fedora)
    if not self.distro.is_fedora():
        self.pkg.install_google_sans_flex()

    # Mark firstrun
    self.fsync.gen_firstrun()
    self.fsync.dedup_and_sort_listfile(INSTALLED_LISTFILE)

    # hyprctl reload
    subprocess.run(["hyprctl", "reload"], check=False)

    self._print_post_install_message()
```

### 4.6 `_install_files_legacy()`

Mirrors `3.files-legacy.sh` exactly:

- MISC: iterate `dots/.config/` entries (excluding quickshell, fish, hypr, fontconfig), `install_dir__sync` each
- MISC: install `dots/.local/share/konsole`
- Quickshell: `install_dir__sync dots/.config/quickshell XDG_CONFIG_HOME/quickshell`
- Fish: `install_dir__sync_exclude dots/.config/fish XDG_CONFIG_HOME/fish ["conf.d"]`
- Fontconfig: install `dots/.config/fontconfig` or `dots-extra/fontsets/{fontset}` if `--fontset`
- Hyprland: sync `dots/.config/hypr/hyprland`, auto_backup for `hyprlock.conf`/`hypridle.conf`/`monitors.conf`/`workspaces.conf`, overwrite for `hyprland.conf` (unless `--skip-hyprland-entry`), ignore_existing for `custom/`
- Icon: install `dots/.local/share/icons/illogical-impulse.svg`

### 4.7 `cmd_install_deps()`, `cmd_install_setups()`, `cmd_install_files()`

Thin wrappers that call the same internal methods but only that specific step.

```python
def cmd_install_deps(self):
    self._prevent_sudo_or_root()
    self.distro.print_info()
    self.sh.pause()
    self.sh.sudo_init_keepalive()
    try:
        self._install_deps()
    finally:
        self.sh.sudo_stop_keepalive()

def cmd_install_setups(self):
    self._prevent_sudo_or_root()
    self.distro.print_info()
    self.sh.pause()
    self.sh.sudo_init_keepalive()
    try:
        self._install_setups()
    finally:
        self.sh.sudo_stop_keepalive()

def cmd_install_files(self):
    self._prevent_sudo_or_root()
    self.distro.print_info()
    self.sh.pause()
    self._install_files()
```

### 4.8 `cmd_uninstall()`

```python
def cmd_uninstall(self):
    self._prevent_sudo_or_root()
    sh = self.sh
    fsync = self.fsync

    sh.log_red("===CAUTION===")
    sh.log_red("This will try to revert changes made by ./setup install.")
    sh.log_red("It is far from enough to precisely revert all changes.")
    sh.log_red("It has not been fully tested. Use at your own risk.")
    sh.pause()

    # Step 3 undo: delete installed files
    if INSTALLED_LISTFILE.exists():
        fsync.interactive_deletion(INSTALLED_LISTFILE)

    # Clean empty dirs under XDG_CONFIG_HOME and XDG_DATA_HOME/konsole
    fsync.clean_empty_dirs([XDG_CONFIG_HOME, XDG_DATA_HOME / "konsole"])

    # Step 2 undo: groups
    user = os.getlogin()
    for group in ["video", "i2c", "input"]:
        sh.run(["sudo", "gpasswd", "-d", user, group], check=False)
    sh.run(["sudo", "rm", "-f", "/etc/modules-load.d/i2c-dev.conf"], check=False)

    # Step 1 undo: uninstall deps
    dist_uninstall = self.repo_root / f"sdata/dist-{self.distro.os_group_id}/uninstall-deps.sh"
    if dist_uninstall.exists():
        # Read packages.conf, run yay -Rns for each (arch)
        # or dnf history undo for each transaction (fedora)
        if self.distro.is_arch():
            self.pkg._uninstall_arch()
        elif self.distro.is_fedora():
            self.pkg._uninstall_fedora()
    else:
        sh.log_warning("No uninstall script for this distro.")

    sh.log_info(f"Hint: Backups under {BACKUP_DIR}")
```

### 4.9 `cmd_diagnose()`

```python
def cmd_diagnose(self):
    output = []  # list of (section, lines)

    def section(title: str):
        output.append(f"\n{'=' * (len(title) + 2)}")
        output.append(f"  {title}")
        output.append(f"{'=' * (len(title) + 2)}")

    def run_diag(cmd: list[str]) -> str:
        try:
            r = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
            return r.stdout if r.returncode == 0 else f"[EXIT {r.returncode}] {r.stderr}"
        except Exception as e:
            return f"[ERROR] {e}"

    section("Git repo info")
    for cmd in [["git", "remote", "get-url", "origin"],
                ["git", "rev-parse", "HEAD"],
                ["git", "status"],
                ["git", "submodule", "status", "--recursive"]]:
        output.append(f"$ {' '.join(cmd)}\n{run_diag(cmd)}\n")

    section("Distro")
    output.append(f"Distro ID: {self.distro.os_distro_id}")
    output.append(f"Distro ID_LIKE: {self.distro.os_distro_id_like}")
    output.append(run_diag(["cat", "/etc/os-release"]))

    section("Variables")
    for name, val in [("XDG_CACHE_HOME", XDG_CACHE_HOME), ...]:
        output.append(f"{name}={val}")

    section("Directories/files")
    output.append(run_diag(["ls", "-l", str(ILLOGICAL_IMPULSE_VIRTUAL_ENV)]))

    section("Versions")
    for cmd in [["Hyprland", "--version"]]:
        output.append(run_diag(cmd))
    output.append(run_diag(["pacman", "-Q"]) + " | grep quickshell" )  # filter PACMAN

    # Write output
    result_path = self.repo_root / "diagnose.result"
    result_path.write_text("\n".join(output))
    print(f"\nOutput saved as \"{result_path}\"")

    # Pastebin upload (optional)
    if not getattr(self.args, 'no_upload', False):
        if self.sh.ask_yes_no("Upload to 0x0.st pastebin? (public, 15-day expiry) [y/N]", default=False):
            try:
                import urllib.request, urllib.parse
                # Use urllib since we're stdlib-only
                # curl -F'file=@diagnose.result' https://0x0.st
                ...
```

### 4.10 `cmd_resetfirstrun()`

```python
def cmd_resetfirstrun(self):
    if FIRSTRUN_FILE.exists():
        FIRSTRUN_FILE.unlink()
        print(f"Deleted {FIRSTRUN_FILE}")
    else:
        print(f"{FIRSTRUN_FILE} does not exist, nothing to do.")
```

### 4.11 `cmd_checkdeps()`

```python
def cmd_checkdeps(self):
    """Check whether packages in packages.conf exist in AUR or Arch repos."""
    import urllib.request
    import gzip

    pkg_file = getattr(self.args, 'list_file', None) or \
               self.repo_root / "sdata/dist-arch/packages.conf"

    packages = PackageManager.read_packages_conf(pkg_file)

    # Fetch AUR package list
    url = "https://aur.archlinux.org/packages.gz"
    resp = urllib.request.urlopen(url)
    aur_packages = set(gzip.decompress(resp.read()).decode().splitlines())
    aur_packages.discard("")  # remove empty header line

    # Get local repo packages
    result = subprocess.run(["pacman", "-Ssq"], capture_output=True, text=True)
    repo_packages = set(result.stdout.splitlines())

    all_available = aur_packages | repo_packages
    missing = sorted(p for p in packages if p not in all_available)

    if missing:
        print("Non-existent packages:")
        for p in missing:
            print(f"  {p}")
    else:
        print("All packages exist in AUR or repos.")
```

### 4.12 Passthrough subcommands (`exp-update`, `exp-merge`, `virtmon`)

```python
def cmd_passthrough(self, subcommand: str, extra_args: list[str]):
    script = self.repo_root / f"sdata/subcmd-{subcommand}/0.run.sh"
    if not script.exists():
        print(f"Error: {script} not found")
        sys.exit(1)

    # Source the lib files + the subcommand script
    # This requires running via bash since the scripts use bash-specific features
    lib_dir = self.repo_root / "sdata/lib"
    cmd = ["bash", "-c",
           f"cd {self.repo_root} && "
           f"source {lib_dir}/environment-variables.sh && "
           f"source {lib_dir}/functions.sh && "
           f"source {lib_dir}/package-installers.sh && "
           f"source {lib_dir}/dist-determine.sh && "
           f"source {script}"]
    # Note: For now, the lib files still exist. After full migration,
    # only the passthrough subcommands' directories need to keep their .sh files.
    os.execvp("bash", cmd)
```

---

## 5. Which Bash Scripts Get Deleted vs Kept

### Delete (replaced by `setup.py`)

```
setup
diagnose
sdata/lib/environment-variables.sh
sdata/lib/functions.sh
sdata/lib/package-installers.sh
sdata/lib/dist-determine.sh
sdata/dist-arch/install-deps.sh
sdata/dist-arch/uninstall-deps.sh
sdata/dist-fedora/install-deps.sh
sdata/dist-fedora/uninstall-deps.sh
sdata/subcmd-install/            (entire directory: options.sh, 0.greeting.sh, 1.deps-router.sh, 2.setups.sh, 3.files.sh, 3.files-legacy.sh, 3.files-exp.sh)
sdata/subcmd-uninstall/          (entire directory)
sdata/subcmd-checkdeps/          (entire directory)
sdata/subcmd-resetfirstrun/       (entire directory)
```

### Keep

```
sdata/subcmd-exp-update/          (0.run.sh, options.sh, exp-update-tester.sh)
sdata/subcmd-virtmon/             (0.run.sh, options.sh, hypr_mon_guard)
sdata/subcmd-merge/               (0.run.sh, options.sh)
sdata/dist-arch/packages.conf
sdata/dist-arch/illogical-impulse-microtex-git/   (PKGBUILD etc.)
sdata/dist-fedora/feddeps.toml
sdata/dist-fedora/SPECS/
sdata/dist-fedora/user_data.yaml
sdata/uv/requirements.txt
sdata/subcmd-install/3.files-exp.yaml
dots/
dots-extra/
os-release
```

**Note on passthrough**: When `setup.py` handles `exp-update`/`exp-merge`/`virtmon`, it will execute the bash scripts directly. The `sdata/lib/*.sh` files needed by the kept bash scripts must either remain (if the kept scripts `source` them) or the kept scripts must be updated to be self-contained. Since `exp-update`, `exp-merge`, and `virtmon` all source `sdata/lib/functions.sh` and `dist-determine.sh`, **those lib files must remain** as long as the passthrough scripts exist, OR the passthrough scripts must be modified to inline what they need.

**Recommendation**: Keep `sdata/lib/` directory for now since the passthrough scripts depend on it. The `setup.py` wrapper will source these lib files before executing the passthrough script. Delete `setup`, `diagnose`, and the fully-migrated subcommand directories only.

**Revised deletion list** (preserve `sdata/lib/`):

```
setup                    → replaced by thin bash wrapper calling python3 setup.py
diagnose                 → replaced by setup.py diagnose
sdata/subcmd-install/    → replaced by setup.py
sdata/subcmd-uninstall/  → replaced by setup.py
sdata/subcmd-checkdeps/  → replaced by setup.py
sdata/subcmd-resetfirstrun/ → replaced by setup.py
sdata/dist-arch/install-deps.sh   → replaced by setup.py
sdata/dist-arch/uninstall-deps.sh → replaced by setup.py
sdata/dist-fedora/install-deps.sh → replaced by setup.py
sdata/dist-fedora/uninstall-deps.sh → replaced by setup.py
```

**Keep** (needed by passthrough scripts):

```
sdata/lib/environment-variables.sh
sdata/lib/functions.sh
sdata/lib/package-installers.sh
sdata/lib/dist-determine.sh
```

---

## 6. Migration Notes

### 6.1 `--force` replaces interactive `v()`/`x()`/`ask`

The bash scripts have a dual-mode system:

- `ask=true` → every command shown with `v()`, user must confirm y/n/s/yesforall
- `ask=false` (set by `--force`) → commands run via `x()`, on failure offer retry/ignore/exit

In `setup.py`:

- **Interactive mode** (`--force` not given): `Shell.run()` shows command, prompts on failure for retry/ignore/exit.
- **Force mode** (`--force` given): `Shell.run()` runs commands, exits on failure (like `set -e`).

### 6.2 TOML parsing for Fedora

Python 3.11+ includes `tomllib`. For 3.9-3.10 compatibility, use a simple fallback: parse `feddeps.toml` with regex or vendor `tomllib`. Since the codebase targets modern distros (Arch, Fedora 42+), Python 3.11+ is a safe assumption. If needed, we can vendor a minimal TOML parser (~50 lines) or require `tomllib`/`tomli`.

### 6.3 YAML parsing for experimental file patterns

The `3.files-exp.yaml` file requires YAML parsing. Since we're stdlib-only, use the approach from CPython's `Lib/test/test_json`: either:

1. Vendor a minimal YAML parser (~200 lines covering the subset used here), or
2. Shell out to `yq` (already a dependency on Fedora, and available in AUR for Arch), or
3. Convert `3.files-exp.yaml` to `3.files-exp.json` and use `json` stdlib module.

**Recommendation**: Use `subprocess.run(["yq", ...])` since `yq` is already a required dependency. This avoids vendoring and stays simple.

### 6.4 `os-release` custom override

The bash scripts check for `${REPO_ROOT}/os-release` before `/etc/os-release`. The Python `Distro.detect()` method must do the same:

```python
def detect(self):
    custom = self.repo_root / "os-release"
    if custom.exists():
        os_release = custom
    elif Path("/etc/os-release").exists():
        os_release = Path("/etc/os-release")
    else:
        self.sh.log_die("/etc/os-release does not exist")

    # Parse KEY=VALUE pairs
    ...
```

### 6.5 Installed file tracking

The bash scripts use `${INSTALLED_LISTFILE}` (a plain text file with one path per line) to track installed files for uninstallation. `setup.py` will maintain this same format so `INSTALLED_LISTFILE` stays backwards-compatible.

### 6.6 Sudo keepalive

Python `threading.Timer` replaces the bash background process:

```python
class SudoKeepalive(threading.Thread):
    def __init__(self):
        super().__init__(daemon=True)
        self._stop = threading.Event()

    def run(self):
        while not self._stop.wait(55):
            subprocess.run(["sudo", "-v"], capture_output=True)

    def stop(self):
        self._stop.set()
```

### 6.7 The `r()` function in Fedora installer

The Fedora `install-deps.sh` uses a `r()` wrapper that records DNF transaction IDs to `user_data.yaml`. This requires:

1. Before each `dnf` operation, get current transaction ID
2. After each `dnf` operation, check if a new transaction was created
3. Record new transaction IDs in `user_data.yaml`

In Python, this becomes:

```python
def _dnf_with_transaction_record(self, cmd: list[str]):
    """Run a dnf command and record any new transaction ID to user_data.yaml."""
    old_id = self._get_latest_dnf_transaction_id()
    self.sh.run(cmd)
    new_id = self._get_latest_dnf_transaction_id()
    if old_id != new_id and new_id:
        self._record_dnf_transaction(new_id)
```

### 6.8 Thread safety and file I/O

- `INSTALLED_LISTFILE` writes are append-only during install; no locking needed for single-process execution.
- `user_data.yaml` writes (Fedora DNF transaction tracking) should read-modify-write atomically.

### 6.9 Testing strategy

1. **Dry-run mode**: Add `--dry-run` flag that prints commands instead of executing them.
2. **Unit tests**: `Distro.detect()`, `PackageManager.read_packages_conf()`, `FileSync` path operations.
3. **Integration test**: Run `python3 setup.py install --force --skip-alldeps --skip-allsetups` in a container.

### 6.10 Entry point

```python
def main():
    parser = argparse.ArgumentParser(prog="setup", description="illogical-impulse setup")
    subparsers = parser.add_subparsers(dest="subcommand", required=True)

    # install subparser
    p_install = subparsers.add_parser("install")
    p_install.add_argument("-f", "--force", action="store_true")
    p_install.add_argument("-F", "--firstrun", action="store_true")
    p_install.add_argument("-c", "--clean", action="store_true")
    # ... all --skip-* flags ...

    # install-deps, install-setups, install-files subparsers
    # (subset of install flags)

    # uninstall, diagnose, resetfirstrun, checkdeps subparsers

    # Passthrough: exp-update, exp-merge, virtmon
    p = subparsers.add_parser("exp-update")
    p.add_argument("remaining", nargs=argparse.REMAINDER)
    # ... similarly for exp-merge, virtmon

    args = parser.parse_args()

    # Prevent running as root
    if os.geteuid() == 0:
        print("This script must NOT be run as root or with sudo.")
        sys.exit(1)

    # Route to subcommand
    setup = Setup(args)
    {
        "install":        setup.cmd_install,
        "install-deps":   setup.cmd_install_deps,
        "install-setups": setup.cmd_install_setups,
        "install-files":  setup.cmd_install_files,
        "uninstall":      setup.cmd_uninstall,
        "diagnose":       setup.cmd_diagnose,
        "resetfirstrun":  setup.cmd_resetfirstrun,
        "checkdeps":      setup.cmd_checkdeps,
        "exp-update":     lambda: setup.cmd_passthrough("exp-update", args.remaining),
        "exp-merge":      lambda: setup.cmd_passthrough("exp-merge", args.remaining),
        "virtmon":         lambda: setup.cmd_passthrough("virtmon", args.remaining),
    }[args.subcommand]()

if __name__ == "__main__":
    main()
```

### 6.11 The `setup` wrapper script

After creating `setup.py`, replace the `setup` bash file with:

```bash
#!/usr/bin/env bash
cd "$(dirname "$0")"
exec python3 setup.py "$@"
```

This maintains the `./setup install` workflow. The old bash `setup` file gets deleted; this new 3-line wrapper takes its place.

### 6.12 Edge cases handled

1. **DBUS_SESSION_BUS_ADDRESS missing**: When `systemctl --user` won't work, fall back to `sudo systemctl --machine=$(whoami)@.host --user`.
2. **Fedora-specific**: `uinput` module, polkit agent, `video+input` groups.
3. **Outdate detection**: Read `outdate-detect-mode` file in dist dirs; compare git timestamps.
4. **Shallow clone**: `git fetch --unshallow` if `.git/shallow` exists.
5. **Architecture check**: Warn if not x86_64.
6. **Custom os-release**: `${REPO_ROOT}/os-release` override.
