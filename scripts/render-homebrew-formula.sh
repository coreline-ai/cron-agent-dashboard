#!/usr/bin/env bash
# render-homebrew-formula.sh — fill the Homebrew formula template with the
# release tag/version and per-platform SHA256 sums.
#
# Usage:
#   ./scripts/render-homebrew-formula.sh <release-version> <sha256sums-path> <output-path>
#
# <release-version> is the exact tag / ldflags value used for the GitHub
# Release (for example "v0.1.0"). The rendered formula keeps that exact value
# in release download URLs and in the binary smoke test, while stripping a
# leading "v" for Homebrew's Formula#version field.
#
# <sha256sums-path> is the SHA256SUMS file produced by the release workflow
# (one "<sha256>  <filename>" line per artifact). The script extracts the
# sums for cron-agent-dashboard-{darwin,linux}-{arm64,amd64} and writes the
# rendered formula to <output-path>. Missing sums for any platform abort
# so the release stays honest — incomplete tap entries silently drop
# arches, which is a footgun.
set -euo pipefail

RELEASE_VERSION="${1:-}"
SUMS="${2:-}"
OUT="${3:-}"
if [[ -z "${RELEASE_VERSION}" || -z "${SUMS}" || -z "${OUT}" ]]; then
  echo "usage: $0 <release-version> <sha256sums-path> <output-path>" >&2
  exit 2
fi
if [[ ! -f "${SUMS}" ]]; then
  echo "render-homebrew-formula: sums file not found: ${SUMS}" >&2
  exit 2
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMPL="${ROOT}/docs/homebrew/cron-agent-dashboard.rb.tmpl"
if [[ ! -f "${TMPL}" ]]; then
  echo "render-homebrew-formula: template not found: ${TMPL}" >&2
  exit 2
fi

lookup_sum() {
  local name="$1"
  local sum
  sum=$(awk -v name="${name}" '$2 == name || $2 == "*"name {print $1}' "${SUMS}" | head -n1)
  if [[ -z "${sum}" ]]; then
    echo "render-homebrew-formula: missing sha256 for ${name} in ${SUMS}" >&2
    exit 1
  fi
  printf '%s' "${sum}"
}

SHA_DARWIN_ARM64=$(lookup_sum cron-agent-dashboard-darwin-arm64)
SHA_DARWIN_AMD64=$(lookup_sum cron-agent-dashboard-darwin-amd64)
SHA_LINUX_ARM64=$(lookup_sum cron-agent-dashboard-linux-arm64)
SHA_LINUX_AMD64=$(lookup_sum cron-agent-dashboard-linux-amd64)
FORMULA_VERSION="${RELEASE_VERSION#v}"

sed \
  -e "s|{{TAG}}|${RELEASE_VERSION}|g" \
  -e "s|{{VERSION}}|${FORMULA_VERSION}|g" \
  -e "s|{{CLI_VERSION}}|${RELEASE_VERSION}|g" \
  -e "s|{{SHA256_DARWIN_ARM64}}|${SHA_DARWIN_ARM64}|g" \
  -e "s|{{SHA256_DARWIN_AMD64}}|${SHA_DARWIN_AMD64}|g" \
  -e "s|{{SHA256_LINUX_ARM64}}|${SHA_LINUX_ARM64}|g" \
  -e "s|{{SHA256_LINUX_AMD64}}|${SHA_LINUX_AMD64}|g" \
  "${TMPL}" > "${OUT}"

echo "render-homebrew-formula: wrote ${OUT}"
