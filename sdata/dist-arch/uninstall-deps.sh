# This script is meant to be sourced.
# It's not for directly running.

packages_conf="${REPO_ROOT}/sdata/dist-arch/packages.conf"
mapfile -t pkg_list < <(sed -e 's/#.*//' -e '/^[[:space:]]*$/d' "$packages_conf")

# Uninstall packages from the flat list
for pkg in "${pkg_list[@]}"; do
  v yay -Rns "$pkg"
done

# Uninstall locally built packages
for pkg in illogical-impulse-microtex-git; do
  v yay -Rns "$pkg"
done