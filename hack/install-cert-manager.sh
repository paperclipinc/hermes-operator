#!/usr/bin/env bash
# Install cert-manager into the current kube context for the conformance jobs.
#
# Why a script with retries instead of a single `helm install --wait`?
# cert-manager normally installs in ~15s, but on GitHub's shared runners the
# install intermittently hangs for 20-30+ min (image-pull / webhook-readiness
# flake). A single `helm install --wait --timeout 5m` does not reliably bail out
# of that hang within the conformance job's budget, so the whole job times out
# (see #64). Here each attempt is hard-capped and retried: a stuck attempt is
# killed and the (usually fast) retry succeeds, turning a 30-min hang into a few
# minutes worst case.
set -euo pipefail

CERT_MANAGER_VERSION="${CERT_MANAGER_VERSION:-v1.20.2}"
ATTEMPT_TIMEOUT="${ATTEMPT_TIMEOUT:-4m}"
MAX_ATTEMPTS="${MAX_ATTEMPTS:-4}"

helm repo add jetstack https://charts.jetstack.io >/dev/null 2>&1 || true
helm repo update jetstack

install_once() {
  # `timeout` kills helm if a single attempt wedges; --wait bounds it too.
  timeout "${ATTEMPT_TIMEOUT}" \
    helm upgrade --install cert-manager jetstack/cert-manager \
      --version "${CERT_MANAGER_VERSION}" \
      --namespace cert-manager --create-namespace \
      --set crds.enabled=true \
      --wait --timeout "${ATTEMPT_TIMEOUT}"
}

for attempt in $(seq 1 "${MAX_ATTEMPTS}"); do
  echo "::group::cert-manager install attempt ${attempt}/${MAX_ATTEMPTS}"
  if install_once; then
    echo "::endgroup::"
    echo "cert-manager installed on attempt ${attempt}."
    exit 0
  fi
  echo "attempt ${attempt} did not complete within ${ATTEMPT_TIMEOUT}; retrying..." >&2
  kubectl get pods -n cert-manager -o wide || true
  echo "::endgroup::"
done

echo "cert-manager failed to install after ${MAX_ATTEMPTS} attempts." >&2
kubectl get pods -n cert-manager -o wide || true
exit 1
