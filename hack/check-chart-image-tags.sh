#!/usr/bin/env bash
# Assert that every image reference the chart renders at its own defaults is
# actually pullable from the registry.
#
# Regression guard for #113: release-please used to write a bare version into
# values.yaml `image.tag` while the release workflow only publishes v-prefixed
# tags, so `helm install` at the chart's own defaults produced ImagePullBackOff.
#
# Uses the anonymous registry token + manifest HEAD, so it needs no credentials
# for public images.
set -euo pipefail

CHART_DIR="${CHART_DIR:-charts/hermes-operator}"
APP_VERSION="$(sed -nE 's/^appVersion:[[:space:]]*"?([^"[:space:]]+)"?/\1/p' "${CHART_DIR}/Chart.yaml")"

fail=0

# Resolve a ghcr.io reference to a manifest, following the OCI auth dance.
check_ref() {
  local ref="$1"
  local repo tag host path token code

  host="${ref%%/*}"
  path="${ref#*/}"
  repo="${path%:*}"
  tag="${path##*:}"

  if [[ "$host" != "ghcr.io" ]]; then
    echo "  SKIP $ref (only ghcr.io is checked)"
    return 0
  fi

  # The operator image is only ever published v-prefixed. A bare semver here is
  # the #113 regression: it renders an unpullable reference. Catch it by shape,
  # so it fails even before the tag would have had a chance to exist.
  if [[ "$repo" == "paperclipinc/hermes-operator" && "$tag" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "  FAIL $ref (bare version tag; the release workflow publishes v${tag})"
    fail=1
    return 0
  fi

  # The operator image is published by the release workflow, which runs *after*
  # release-please bumps appVersion. On a release PR (and on main until the
  # release job finishes) the chart legitimately renders a tag that does not
  # exist yet. Exempt exactly that case from the existence check, but still
  # enforce the v-prefix format that #113 was about.
  if [[ "$repo" == "paperclipinc/hermes-operator" && "$tag" == "v${APP_VERSION}" ]]; then
    if ! git rev-parse -q --verify "refs/tags/v${APP_VERSION}" >/dev/null 2>&1; then
      echo "  SKIP $ref (v${APP_VERSION} is the pending release; format is correct)"
      return 0
    fi
  fi

  token=$(curl -fsSL "https://ghcr.io/token?scope=repository:${repo}:pull&service=ghcr.io" \
    | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])')

  code=$(curl -s -o /dev/null -w '%{http_code}' -I \
    -H "Authorization: Bearer ${token}" \
    -H 'Accept: application/vnd.oci.image.index.v1+json' \
    -H 'Accept: application/vnd.oci.image.manifest.v1+json' \
    -H 'Accept: application/vnd.docker.distribution.manifest.list.v2+json' \
    -H 'Accept: application/vnd.docker.distribution.manifest.v2+json' \
    "https://ghcr.io/v2/${repo}/manifests/${tag}")

  if [[ "$code" == "200" ]]; then
    echo "  OK   $ref"
  else
    echo "  FAIL $ref (HTTP $code — tag does not resolve)"
    fail=1
  fi
}

echo "Rendering ${CHART_DIR} at its defaults..."
refs=$(helm template hermes-operator "${CHART_DIR}" \
  | grep -oE 'image: "?[a-z0-9./-]+:[A-Za-z0-9._-]+"?' \
  | sed -E 's/^image: "?//; s/"?$//' \
  | sort -u)

if [[ -z "$refs" ]]; then
  echo "ERROR: no image references found in rendered chart — check the grep." >&2
  exit 1
fi

echo "Checking rendered image references:"
while IFS= read -r ref; do
  [[ -n "$ref" ]] && check_ref "$ref"
done <<<"$refs"

if [[ "$fail" -ne 0 ]]; then
  echo
  echo "One or more chart default image tags are unpullable." >&2
  echo "The release workflow publishes v-prefixed tags (v<version>), so the" >&2
  echo "chart must render those — see hack/check-chart-image-tags.sh header." >&2
  exit 1
fi

echo "All chart default image tags resolve."
