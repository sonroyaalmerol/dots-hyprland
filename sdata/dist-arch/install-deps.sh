# This script is meant to be sourced.
# It's not for directly running.

install-yay(){
  x sudo pacman -S --needed --noconfirm base-devel
  x git clone https://aur.archlinux.org/yay-bin.git /tmp/buildyay
  x cd /tmp/buildyay
  x makepkg -o
  x makepkg -se
  x makepkg -i --noconfirm
  x cd ${REPO_ROOT}
  rm -rf /tmp/buildyay
}

install-local-pkgbuild() {
  local location=$1
  local installflags=$2

  x pushd $location

  source ./PKGBUILD
  x yay -S --sudoloop $installflags --asdeps "${depends[@]}"
  x makepkg -Afsi --noconfirm
  x popd
}

#####################################################################################
if ! command -v pacman >/dev/null 2>&1; then
  printf "${STY_RED}[$0]: pacman not found, it seems that the system is not ArchLinux or Arch-based distros. Aborting...${STY_RST}\n"
  exit 1
fi

# Keep makepkg from resetting sudo credentials
if [[ -z "${PACMAN_AUTH:-}" ]]; then
  export PACMAN_AUTH="sudo"
fi

case $SKIP_SYSUPDATE in
  true) sleep 0;;
  *) v sudo pacman -Syu;;
esac

if ! command -v yay >/dev/null 2>&1;then
  echo -e "${STY_YELLOW}[$0]: \"yay\" not found.${STY_RST}"
  showfun install-yay
  v install-yay
fi

# Install packages from packages.conf
packages_conf="${REPO_ROOT}/sdata/dist-arch/packages.conf"
mapfile -t pkg_list < <(sed -e 's/#.*//' -e '/^[[:space:]]*$/d' "$packages_conf")
v yay -S --needed --noconfirm "${pkg_list[@]}"

# Build and install MicroTeX from local PKGBUILD
showfun install-local-pkgbuild
metainstallflags="--needed"
$ask && metainstallflags="" || metainstallflags="$metainstallflags --noconfirm"
v install-local-pkgbuild "./sdata/dist-arch/illogical-impulse-microtex-git" "$metainstallflags"