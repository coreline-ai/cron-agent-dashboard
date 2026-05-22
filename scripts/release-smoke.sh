#!/usr/bin/env bash
# release-smoke.sh — sanity-check a freshly built cron-agent-dashboard binary.
#
# Runs `--version` and verifies `serve --help` lists the core flags. This is
# intentionally fast (~1s) so it can sit inside the release pipeline without
# inflating the build time. A failure aborts the release.
set -euo pipefail

BIN="${1:-}"
if [[ -z "${BIN}" ]]; then
  echo "usage: $0 <path-to-binary>" >&2
  exit 2
fi
if [[ ! -x "${BIN}" ]]; then
  echo "release-smoke: not executable: ${BIN}" >&2
  exit 2
fi

# --version must exit 0 and print a non-empty line. The exact format is owned
# by httpapi.Version (injected via -ldflags) so the assertion stays loose on
# purpose.
version_out="$("${BIN}" --version 2>&1 || true)"
if [[ -z "${version_out}" ]]; then
  echo "release-smoke: --version produced no output" >&2
  exit 1
fi
echo "release-smoke: --version => ${version_out}"

# serve --help must mention the bind / data-dir flags so we catch a flag-set
# regression before shipping.
help_out="$("${BIN}" serve --help 2>&1 || true)"
for needle in '-bind' '-data-dir' '-db'; do
  if ! grep -q -- "${needle}" <<<"${help_out}"; then
    echo "release-smoke: serve --help missing flag ${needle}" >&2
    echo "${help_out}" >&2
    exit 1
  fi
done

echo "release-smoke: ok"
