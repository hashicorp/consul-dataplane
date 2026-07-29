#!/usr/bin/env bash
# Copyright IBM Corp. 2022, 2026
# SPDX-License-Identifier: MPL-2.0
#
# Triggers a consul-dataplane release promotion to staging or production via `bob`.
#
# Usage:
#   ./release-scripts/trigger-promotion.sh [staging|production] [options]
#
# The promotion target defaults to "staging" when no argument is provided.
#
# Options:
#   -n, --dry-run   Resolve all inputs and print the exact `bob` command WITHOUT
#                   executing it (also enabled by setting DRY_RUN=true).
#   -y, --yes       Non-interactive: skip the prompts/confirmation and use the
#                   values from the environment (or the derived defaults).
#   -h, --help      Show this help and exit.
#
# Variable names are kept consistent with prepare-release-branch.sh. Without -y,
# the script prompts for each setting below, showing the current env value (or a
# derived default) that you can accept (press Enter) or override:
#   DP_RELEASE_VERSION, DP_PRODUCT_VERSION, DP_RELEASE_BRANCH, REMOTE
#
# Derived defaults (used when the corresponding env var is unset):
#   DP_PRODUCT_VERSION = DP_RELEASE_VERSION
#   DP_RELEASE_BRANCH  = release/<DP_RELEASE_VERSION>
#   REMOTE             = origin
# The release-branch commit SHA (DP_RELEASE_SHA) is resolved automatically from
# <REMOTE>/<DP_RELEASE_BRANCH>.

set -euo pipefail

usage() {
  sed -n '/^# Triggers /,/^set /p' "$0" | sed '/^set /d; s/^# \{0,1\}//; s/^#$//'
}

# -----------------------------------------------------------------------------
# Flags / arguments (promotion target plus dry-run / non-interactive switches)
# -----------------------------------------------------------------------------
DRY_RUN="${DRY_RUN:-false}"
INTERACTIVE=true
PROMOTION_TARGET=""
for arg in "$@"; do
  case "${arg}" in
    -n | --dry-run) DRY_RUN=true ;;
    -y | --yes) INTERACTIVE=false ;;
    -h | --help)
      usage
      exit 0
      ;;
    staging | production) PROMOTION_TARGET="${arg}" ;;
    *)
      echo "Unknown argument: ${arg}" >&2
      usage >&2
      exit 1
      ;;
  esac
done

# Normalize DRY_RUN to strictly "true" or "false" (accepts true/1/yes from env).
case "${DRY_RUN}" in
  true | TRUE | True | 1 | yes | YES) DRY_RUN=true ;;
  *) DRY_RUN=false ;;
esac

PROMOTION_TARGET="${PROMOTION_TARGET:-staging}"

# run CMD...  Runs CMD, or in dry-run mode prints it (shell-quoted) instead.
run() {
  if [[ "${DRY_RUN}" == "true" ]]; then
    { printf '  [dry-run] $'; printf ' %q' "$@"; printf '\n'; }
  else
    "$@"
  fi
}

# fail_or_warn MESSAGE  Errors out normally; in dry-run only warns so the preview
# can run to completion.
fail_or_warn() {
  if [[ "${DRY_RUN}" == "true" ]]; then
    echo "Warning (dry-run): $1" >&2
  else
    echo "Error: $1" >&2
    exit 1
  fi
}

# prompt_var VAR_NAME DEFAULT_VALUE [required]
# Shows the value that will be used and lets the user accept or override it.
prompt_var() {
  local var_name="$1"
  local default_value="$2"
  local required="${3:-}"
  local input
  while :; do
    read -r -p "  ${var_name} [${default_value}]: " input || true
    input="${input:-${default_value}}"
    if [[ -z "${input}" && "${required}" == "required" ]]; then
      echo "  ${var_name} is required; please enter a value." >&2
      continue
    fi
    break
  done
  printf -v "${var_name}" '%s' "${input}"
}

REMOTE="${REMOTE:-origin}"

# -----------------------------------------------------------------------------
# Collect / confirm settings (names kept consistent with prepare-release-branch.sh)
# -----------------------------------------------------------------------------
if [[ "${INTERACTIVE}" == "true" ]]; then
  echo "Confirm promotion settings (press Enter to keep the shown value, or type a new one):"
  echo
fi

# DP_RELEASE_VERSION comes first because the other defaults derive from it.
DP_RELEASE_VERSION="${DP_RELEASE_VERSION:-}"
if [[ "${INTERACTIVE}" == "true" ]]; then
  prompt_var DP_RELEASE_VERSION "${DP_RELEASE_VERSION}" required
fi
: "${DP_RELEASE_VERSION:?DP_RELEASE_VERSION is required (set it or run interactively)}"

# Derive defaults from the (possibly just-entered) version.
DP_PRODUCT_VERSION="${DP_PRODUCT_VERSION:-${DP_RELEASE_VERSION}}"
DP_RELEASE_BRANCH="${DP_RELEASE_BRANCH:-release/${DP_RELEASE_VERSION}}"

if [[ "${INTERACTIVE}" == "true" ]]; then
  prompt_var DP_PRODUCT_VERSION "${DP_PRODUCT_VERSION}" required
  prompt_var DP_RELEASE_BRANCH  "${DP_RELEASE_BRANCH}"  required
  prompt_var REMOTE             "${REMOTE}"             required
  echo
fi

# Export so child processes (e.g. bob) inherit the resolved values.
export DP_RELEASE_VERSION DP_PRODUCT_VERSION DP_RELEASE_BRANCH REMOTE

# -----------------------------------------------------------------------------
# Prerequisite checks (git is always required; bob only for a real promotion)
# -----------------------------------------------------------------------------
if ! command -v git >/dev/null 2>&1; then
  echo "Error: required command 'git' not found in PATH." >&2
  exit 1
fi
if ! command -v bob >/dev/null 2>&1; then
  fail_or_warn "required command 'bob' not found in PATH."
fi

# Ensure the remote ref is current so we resolve the latest commit SHA. This is a
# read-only operation, so it runs even in dry-run to print an accurate command.
echo "Fetching latest refs for ${DP_RELEASE_BRANCH} from ${REMOTE}..."
if ! git fetch "${REMOTE}" "${DP_RELEASE_BRANCH}"; then
  fail_or_warn "'git fetch' failed for ${DP_RELEASE_BRANCH}."
fi

# Resolve the latest commit SHA of the release branch.
if ! DP_RELEASE_SHA="$(git rev-parse "${REMOTE}/${DP_RELEASE_BRANCH}" 2>/dev/null)"; then
  fail_or_warn "unable to resolve ${REMOTE}/${DP_RELEASE_BRANCH}. Does the branch exist on ${REMOTE}?"
  DP_RELEASE_SHA="<unresolved-sha-for-${REMOTE}/${DP_RELEASE_BRANCH}>"
fi
export DP_RELEASE_SHA

# -----------------------------------------------------------------------------
# Print configuration and confirm
# -----------------------------------------------------------------------------
cat <<EOF

The following variables are set:

  DP_RELEASE_VERSION                   = ${DP_RELEASE_VERSION}
  DP_PRODUCT_VERSION                   = ${DP_PRODUCT_VERSION}
  DP_RELEASE_BRANCH                    = ${DP_RELEASE_BRANCH}
  DP_RELEASE_SHA                       = ${DP_RELEASE_SHA}
  REMOTE                               = ${REMOTE}

  Promotion target                     = ${PROMOTION_TARGET}
  Dry run                              = ${DRY_RUN}

EOF

# -----------------------------------------------------------------------------
# Build the promotion command (single source of truth for run + dry-run)
# -----------------------------------------------------------------------------
promotion_cmd=(
  bob trigger-promotion
  --product-name=consul-dataplane
  --repo=consul-dataplane
  --product-version="${DP_PRODUCT_VERSION}"
  --sha="${DP_RELEASE_SHA}"
  # CRT/releases-api environment for consul-dataplane (OSS-only product).
  --environment=consul-dataplane-oss
  # feed-consul-ci; matches notification_channel in .release/release-metadata.hcl.
  --slack-channel=C09KX8B2KC6
  --org hashicorp
  --branch "${DP_RELEASE_BRANCH}"
  "${PROMOTION_TARGET}"
)

if [[ "${DRY_RUN}" == "true" ]]; then
  echo ">>> DRY RUN: no promotion will be triggered."
  echo ">>> The command that would run is printed below, prefixed with [dry-run]."
  echo
elif [[ "${INTERACTIVE}" == "true" ]]; then
  read -r -p "Proceed with promotion to ${PROMOTION_TARGET}? [y/N] " response || true
  case "${response}" in
    [yY] | [yY][eE][sS]) ;;
    *)
      echo "Aborted. No promotion was triggered."
      exit 1
      ;;
  esac
fi

# -----------------------------------------------------------------------------
# Trigger the promotion (printed in dry-run, executed otherwise)
# -----------------------------------------------------------------------------
echo "==> Triggering promotion to ${PROMOTION_TARGET}..."
run "${promotion_cmd[@]}"

if [[ "${DRY_RUN}" == "true" ]]; then
  echo
  echo "Dry run complete. No promotion was triggered."
fi
