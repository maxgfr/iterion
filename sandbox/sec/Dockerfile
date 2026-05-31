# syntax=docker/dockerfile:1.7
#
# iterion-sandbox-sec — security-scanner variant layered on
# iterion-sandbox-full. It adds the SAST / SCA / secret scanners the
# sec-audit-source and sec-audit-deps bots shell out to, which neither
# the slim nor full base ships. Select it via:
#
#   --sandbox-default-image ghcr.io/socialgouv/iterion-sandbox-sec:<tag>
#
# (or a workflow `sandbox.image:`). Without it the sec-audit bots run
# their scanner tool-nodes against a host/base that has none of these
# binaries, every scan `|| true`-falls-through, and the audit silently
# reports zero findings — a façade.
#
# Scanners covered:
#   - semgrep   — broad-spectrum SAST (auto + per-language rulesets).
#   - gosec     — Go SAST.
#   - govulncheck — Go module vulnerability DB (SCA).
#   - bandit    — Python SAST.
#   - pip-audit — Python dependency vulnerabilities (SCA).
#   - trivy     — fs vuln/config/secret scanning (SCA + IaC + secrets).
#   - gitleaks  — secret detection.

ARG BASE=ghcr.io/socialgouv/iterion-sandbox-full:edge
FROM ${BASE}

USER root

# ---------------------------------------------------------------------
# Layer 1 — Python scanners via pip. Debian 13 enforces PEP 668, so the
# system install needs --break-system-packages; the console scripts land
# on /usr/local/bin which is already on PATH. semgrep is the heavy one.
# ---------------------------------------------------------------------
RUN pip3 install --no-cache-dir --break-system-packages \
        semgrep \
        bandit \
        pip-audit

# ---------------------------------------------------------------------
# Layer 2 — Go scanners. The apt `go` from the full base drives
# `go install`; GOBIN pins the binaries into /usr/local/bin so they are
# on PATH for every user (the bots run as the `devbox` user).
# ---------------------------------------------------------------------
ARG GOSEC_VERSION=latest
ARG GOVULNCHECK_VERSION=latest
RUN GOBIN=/usr/local/bin GOFLAGS=-mod=mod \
        go install "github.com/securego/gosec/v2/cmd/gosec@${GOSEC_VERSION}" \
 && GOBIN=/usr/local/bin GOFLAGS=-mod=mod \
        go install "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}" \
 && rm -rf /root/go/pkg /root/.cache/go-build

# ---------------------------------------------------------------------
# Layer 3 — single-binary scanners. Arch-derived like the full image so
# the multi-arch buildx pipeline works on amd64 + arm64.
#   trivy    — official install script (handles arch + checksum).
#   gitleaks — release tarball; asset arch suffix is x64 / arm64.
# ---------------------------------------------------------------------
ARG TRIVY_VERSION=0.70.0
ARG GITLEAKS_VERSION=8.21.2
RUN case "$(uname -m)" in \
        x86_64)  TRIVY_ARCH="Linux-64bit"; GL_ARCH="x64" ;; \
        aarch64) TRIVY_ARCH="Linux-ARM64"; GL_ARCH="arm64" ;; \
        *)       echo "unsupported arch $(uname -m)" && exit 1 ;; \
    esac \
 && curl -fsSL "https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_${TRIVY_ARCH}.tar.gz" \
        | tar -xz -C /usr/local/bin trivy \
 && chmod +x /usr/local/bin/trivy \
 && curl -fsSL "https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/gitleaks_${GITLEAKS_VERSION}_linux_${GL_ARCH}.tar.gz" \
        | tar -xz -C /usr/local/bin gitleaks \
 && chmod +x /usr/local/bin/gitleaks

USER devbox
WORKDIR /workspace

# Build-time sanity check — fail the build rather than ship an image
# that silently lacks a scanner (the exact façade this image prevents).
RUN semgrep --version \
 && gosec --version \
 && govulncheck -version \
 && bandit --version \
 && pip-audit --version \
 && trivy --version \
 && gitleaks version
