#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${BIN_DIR:-"$ROOT_DIR/.tools/bin"}"
TMP_DIR="${TMPDIR:-/tmp}/namespace-class-tools"

mkdir -p "$BIN_DIR" "$TMP_DIR"

os="$(uname | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

case "$os" in
  darwin|linux) ;;
  *) echo "Unsupported OS: $os" >&2; exit 1 ;;
esac

install_kubectl() {
  if [[ -x "$BIN_DIR/kubectl" && "${FORCE_TOOLS:-0}" != "1" ]]; then
    "$BIN_DIR/kubectl" version --client >/dev/null
    echo "kubectl already installed at $BIN_DIR/kubectl"
    return
  fi

  local version
  version="${KUBECTL_VERSION:-$(curl -fsSL https://dl.k8s.io/release/stable.txt)}"
  echo "Installing kubectl $version for $os/$arch"
  curl -fL --retry 3 -o "$BIN_DIR/kubectl" "https://dl.k8s.io/release/${version}/bin/${os}/${arch}/kubectl"
  chmod +x "$BIN_DIR/kubectl"
}

install_helm() {
  if [[ -x "$BIN_DIR/helm" && "${FORCE_TOOLS:-0}" != "1" ]]; then
    "$BIN_DIR/helm" version --short >/dev/null
    echo "helm already installed at $BIN_DIR/helm"
    return
  fi

  local version
  version="${HELM_VERSION:-$(curl -fsSL https://api.github.com/repos/helm/helm/releases/latest | sed -n 's/.*"tag_name": "\(v[^"]*\)".*/\1/p' | head -n 1)}"
  if [[ -z "$version" ]]; then
    echo "Unable to determine latest Helm version; set HELM_VERSION=vX.Y.Z" >&2
    exit 1
  fi

  local archive="$TMP_DIR/helm-${version}-${os}-${arch}.tar.gz"
  local unpack_dir="$TMP_DIR/helm-${version}"
  rm -rf "$unpack_dir"
  mkdir -p "$unpack_dir"

  echo "Installing helm $version for $os/$arch"
  curl -fL --retry 3 -o "$archive" "https://get.helm.sh/helm-${version}-${os}-${arch}.tar.gz"
  tar -xzf "$archive" -C "$unpack_dir"
  cp "$unpack_dir/${os}-${arch}/helm" "$BIN_DIR/helm"
  chmod +x "$BIN_DIR/helm"
}

install_kubectl
install_helm

echo "Installed tools:"
"$BIN_DIR/kubectl" version --client
"$BIN_DIR/helm" version --short

