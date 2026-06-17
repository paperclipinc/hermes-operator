# images/hermes-agent/Dockerfile
#
# Two-stage multi-arch build for ghcr.io/paperclipinc/hermes-agent.
# Stage 1 (`builder`) uses the official uv image to resolve and install the pinned
# Python environment into a self-contained venv at /opt/venv. Stage 2 (`runtime`)
# copies only the venv + minimal apt deps and runs as the non-root `hermes` user.
#
# Build args:
#   HERMES_VERSION  : the exact hermes-agent release to package (e.g. "1.4.3").
#                     Required. The CI workflow at .github/workflows/agent-image.yaml
#                     drives this from a matrix.
#   PYTHON_VERSION  : Python interpreter version. Default 3.11.
#   UV_VERSION      : uv tool version. Default 0.11.7 (pinned for reproducibility; keep in lockstep with the relock workflow).
#   TINI_VERSION    : tini release tag (lesson openclaw #471: tini as PID 1).
#
# Output: a runtime image that, with no further configuration, runs
#         `hermes-agent --config ~/.hermes/config.yaml` as UID 1000.

ARG PYTHON_VERSION=3.11
ARG UV_VERSION=0.11.7
ARG TINI_VERSION=v0.19.0
ARG HERMES_VERSION

# ---------- Stage 1: builder ----------
FROM ghcr.io/astral-sh/uv:${UV_VERSION} AS uv

FROM python:${PYTHON_VERSION}-slim-bookworm AS builder
ARG HERMES_VERSION

# uv binary from the dedicated stage above.
COPY --from=uv /uv /uvx /usr/local/bin/

ENV UV_LINK_MODE=copy \
    UV_COMPILE_BYTECODE=1 \
    UV_PYTHON_DOWNLOADS=never \
    VIRTUAL_ENV=/opt/venv \
    UV_PROJECT_ENVIRONMENT=/opt/venv

WORKDIR /build

# git + ca-certificates are needed at lock-resolve time because the lockfile
# pins hermes-agent by git ref (upstream is not on PyPI).
RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy lockfile + project metadata first for layer caching.
COPY pyproject.toml uv.lock ./

# Sync the frozen lockfile. `make agent-image-relock HERMES_VERSION=vX.Y.Z`
# rewrites the @ ref in pyproject.toml and regenerates uv.lock; at build time
# we simply trust the lock. HERMES_VERSION is informational for the labels stage.
RUN --mount=type=cache,target=/root/.cache/uv \
    set -eux; \
    if [ -z "${HERMES_VERSION}" ]; then \
        echo "ERROR: HERMES_VERSION build-arg is required" >&2; exit 1; \
    fi; \
    uv venv /opt/venv; \
    uv sync --frozen --no-dev

# ---------- Stage 1b: web-builder ----------
# Upstream's wheel ships `hermes_cli/web_dist/**` via package-data, but the
# directory only exists after `cd web && npm run build` runs against the
# source tree. uv installs hermes-agent from a git ref (not editable),
# which fetches the source but never executes the JS build, so web_dist/
# is empty in site-packages and `hermes dashboard` serves
# `{"error":"Frontend not built"}` on every route.
#
# Mirror upstream's Dockerfile: clone the same fork ref pinned in
# pyproject.toml, run the production vite build, then copy the resulting
# web_dist/ into the runtime venv below.
ARG HERMES_AGENT_FORK_REPO=https://github.com/aabichou/hermes-agent-src.git
ARG HERMES_AGENT_FORK_REF=fix/packages-find-hermes-cli-subpackages
FROM node:22-bookworm-slim AS web-builder
ARG HERMES_AGENT_FORK_REPO=https://github.com/aabichou/hermes-agent-src.git
ARG HERMES_AGENT_FORK_REF=fix/packages-find-hermes-cli-subpackages
RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
RUN git clone --depth 1 --branch "${HERMES_AGENT_FORK_REF}" "${HERMES_AGENT_FORK_REPO}" hermes-agent
WORKDIR /src/hermes-agent/web
RUN npm ci --no-audit --no-fund
RUN npm run build
# vite's outDir is "../hermes_cli/web_dist", so the assets land at
# /src/hermes-agent/hermes_cli/web_dist/. Confirm the index exists so a
# silent build failure fails this stage instead of producing an image
# that still 404s.
RUN test -f /src/hermes-agent/hermes_cli/web_dist/index.html

# ---------- Stage 2: runtime ----------
FROM python:${PYTHON_VERSION}-slim-bookworm AS runtime
ARG PYTHON_VERSION
ARG TINI_VERSION
ARG HERMES_VERSION
ARG TARGETARCH

# Runtime apt packages:
#   - ffmpeg, ripgrep: hard dependencies of hermes-agent (audio + search)
#   - git, openssh-client: hermes-agent skill installs are git-based
#   - ca-certificates: TLS to platform gateways
#   - tini: PID 1 reaper (lesson openclaw-operator #471)
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ffmpeg \
        ripgrep \
        git \
        openssh-client \
        ca-certificates \
        tini \
    && rm -rf /var/lib/apt/lists/*

# Non-root user matching the StatefulSet's runAsUser=1000 (Plan 1).
RUN groupadd --system --gid 1000 hermes \
    && useradd --system --uid 1000 --gid 1000 --create-home --shell /usr/sbin/nologin hermes \
    && mkdir -p /home/hermes/.hermes \
    && chown -R hermes:hermes /home/hermes

# Copy the resolved venv from the builder stage.
COPY --from=builder --chown=hermes:hermes /opt/venv /opt/venv

# Drop the pre-built web frontend into the installed hermes_cli package.
# uv installed hermes_cli without web_dist/ (the JS build never ran during
# the wheel build); without these assets `hermes dashboard` returns
# {"error":"Frontend not built"} on every page. The web-builder stage
# above produces /src/hermes-agent/hermes_cli/web_dist/ from the same
# fork ref that uv resolves, so the assets are commit-aligned with the
# Python package.
COPY --from=web-builder --chown=hermes:hermes \
    /src/hermes-agent/hermes_cli/web_dist \
    /opt/venv/lib/python${PYTHON_VERSION}/site-packages/hermes_cli/web_dist

# Copy uv binary so the operator's init-uv container can run `uv sync --frozen`.
COPY --from=builder /usr/local/bin/uv /usr/local/bin/uvx /usr/local/bin/

# Copy pyproject.toml + uv.lock to /opt/venv-template/ so the operator's
# init-uv init container can run `uv sync --frozen` against them.
# Fixes: https://github.com/paperclipinc/hermes-operator/issues/68
RUN mkdir -p /opt/venv-template
COPY --from=builder --chown=hermes:hermes /build/pyproject.toml /build/uv.lock /opt/venv-template/
ENV PATH="/opt/venv/bin:${PATH}" \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    HOME=/home/hermes

# Image metadata. The HERMES_VERSION label is the one the operator's autoupdate
# controller (Plan 5) compares against the registry tag.
LABEL org.opencontainers.image.title="hermes-agent" \
      org.opencontainers.image.source="https://github.com/paperclipinc/hermes-operator" \
      org.opencontainers.image.documentation="https://github.com/paperclipinc/hermes-operator/blob/main/images/hermes-agent/README.md" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.vendor="paperclipinc" \
      hermes.agent/version="${HERMES_VERSION}"

USER hermes
WORKDIR /home/hermes

COPY --chown=hermes:hermes entrypoint.sh /usr/local/bin/hermes-entrypoint
RUN chmod +x /usr/local/bin/hermes-entrypoint

# tini as PID 1 to reap any subprocess hermes-agent forks (audio transcoders,
# git operations, skill executors). `--` separates tini's flags from the wrapped
# entrypoint script.
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/hermes-entrypoint"]
CMD ["serve"]
