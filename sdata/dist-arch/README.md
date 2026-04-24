# Install scripts for Arch Linux / CachyOS

- See also [Install scripts | illogical-impulse](https://ii.clsty.link/en/dev/inst-script/)

## Dependency Installation

Dependencies are managed via a flat package list in `packages.conf`.

The `install-deps.sh` script:

1. Installs yay if missing
2. Installs all packages from `packages.conf` via `yay -S --needed --noconfirm`
3. Builds and installs MicroTeX from its local PKGBUILD (the only remaining local build)

## Note

- `packages.conf` contains one package per line with comment-grouped sections
- `bibata-cursor-theme-bin` is installed from AUR as a simpler alternative to the previous local PKGBUILD
- `microtex-git` still requires a local PKGBUILD build due to source patches
