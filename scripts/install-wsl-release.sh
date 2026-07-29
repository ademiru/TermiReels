#!/usr/bin/env bash
set -euo pipefail

repo="ademiru/TermiReels"
asset="termireels-linux-amd64-wsl.tar.gz"
checksum_asset="${asset}.sha256"

if ! grep -qiE 'microsoft|wsl' /proc/sys/kernel/osrelease 2>/dev/null; then
  echo "TermiReels Windows installer must run inside WSL2." >&2
  exit 1
fi
if [[ "$(uname -m)" != "x86_64" ]]; then
  echo "This release currently supports WSL2 on x86-64 Windows only." >&2
  exit 1
fi

for command in curl tar sha256sum; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "Missing required command: $command" >&2
    exit 1
  fi
done

echo "Installing Linux runtime libraries..."
sudo apt-get update
sudo apt-get install -y \
  ca-certificates \
  libasound2t64 \
  libasound2-plugins \
  libatk-bridge2.0-0t64 \
  libgbm1 \
  libgtk-3-0t64 \
  libnss3 \
  libx11-xcb1 \
  libxcomposite1 \
  libxdamage1 \
  libxrandr2

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT
base_url="https://github.com/${repo}/releases/latest/download"

echo "Downloading the latest verified TermiReels WSL package..."
curl --fail --location --silent --show-error \
  "${base_url}/${asset}" --output "${tmp_dir}/${asset}"
curl --fail --location --silent --show-error \
  "${base_url}/${checksum_asset}" --output "${tmp_dir}/${checksum_asset}"
(
  cd "$tmp_dir"
  sha256sum --check "$checksum_asset"
)

tar -xzf "${tmp_dir}/${asset}" -C "$tmp_dir"
payload="${tmp_dir}/termireels"
if [[ ! -x "${payload}/termireels" || ! -f "${payload}/VERSION" ]]; then
  echo "Downloaded package is incomplete." >&2
  exit 1
fi

version="$(tr -d '\r\n' < "${payload}/VERSION")"
if [[ ! "$version" =~ ^[0-9A-Za-z._-]+$ ]]; then
  echo "Downloaded package has an invalid version." >&2
  exit 1
fi

data_root="${XDG_DATA_HOME:-$HOME/.local/share}/termireels"
bin_root="${HOME}/.local/bin"
versions_root="${data_root}/versions"
destination="${versions_root}/${version}"
mkdir -p "$versions_root" "$bin_root"

if [[ ! -e "$destination" ]]; then
  mv "$payload" "$destination"
fi
if [[ ! -x "${destination}/termireels" ]]; then
  echo "Existing installation for ${version} is incomplete; refusing to overwrite it." >&2
  exit 1
fi

next_link="${data_root}/.current-$$"
ln -s "$destination" "$next_link"
mv -Tf "$next_link" "${data_root}/current"

launcher_tmp="${bin_root}/.termireels-$$"
cat >"$launcher_tmp" <<EOF
#!/usr/bin/env bash
set -euo pipefail
app_root="${data_root}/current"
export PATH="\${app_root}/runtime/node/bin:\${PATH}"
if [[ -n "\${WSL_DISTRO_NAME:-}" && -n "\${PULSE_SERVER:-}" ]]; then
  export ALSA_CONFIG_PATH="\${app_root}/runtime/asound-wsl.conf"
fi
exec "\${app_root}/termireels" "\$@"
EOF
chmod 0755 "$launcher_tmp"
mv -f "$launcher_tmp" "${bin_root}/termireels"

echo
echo "TermiReels ${version} installed successfully."
if [[ ":${PATH}:" != *":${bin_root}:"* ]]; then
  echo "Add this line to ~/.bashrc, then reopen Ubuntu:"
  echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
else
  echo "Next step: termireels --login"
  echo "Normal run: termireels --creator-provider"
fi
