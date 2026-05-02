# Maintainer: Snry Shell <snry@shell.dev>
pkgname=snry-shell-qs
pkgver=2.0.0
pkgrel=1
pkgdesc='Snry Shell - Hyprland dotfiles managed by snry-daemon'
arch=('any')
url='https://github.com/sonroyaalmerol/dots-hyprland'
license=('MIT')
depends=('git' 'rsync' 'python' 'uv' 'sudo' 'findutils')
makedepends=('git' 'go')
source=("git+https://github.com/sonroyaalmerol/dots-hyprland.git")
sha256sums=('SKIP')
backup=()
install=snry-shell-qs.install

package() {
  cd "$srcdir/dots-hyprland"

  # Build snry-daemon binary
  go build -o snry-daemon ./cmd/snry-daemon

  # Install snry-daemon binary
  install -Dm755 snry-daemon "$pkgdir/usr/bin/snry-daemon"

  # Install snry-shell wrapper
  install -Dm755 /dev/stdin "$pkgdir/usr/bin/snry-shell" <<'SCRIPT'
#!/bin/bash
exec /usr/bin/snry-daemon setup "$@"
SCRIPT

  # Install shared data
  install -dm755 "$pkgdir/usr/share/snry-shell"
  cp -a configs data frontend "$pkgdir/usr/share/snry-shell/"
}
