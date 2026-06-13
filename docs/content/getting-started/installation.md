---
title: "Installation"
description: "Install goodread from a release, with go install, from a package manager, or as a container."
weight: 20
---

## Prebuilt binaries

Every [release](https://github.com/tamnd/goodread-cli/releases) carries archives
for Linux, macOS, Windows, and FreeBSD on amd64 and arm64, plus deb, rpm, and
apk packages for Linux. Download, unpack, put `goodread` on your `PATH`, done.
The `checksums.txt` on each release is signed with keyless
[cosign](https://docs.sigstore.dev/), and SBOMs ship alongside, if you want to
verify before running.

## With Go

```bash
go install github.com/tamnd/goodread-cli/cmd/goodread@latest
```

That puts `goodread` in `$(go env GOPATH)/bin`, which is `~/go/bin` unless you
moved it. Make sure that directory is on your `PATH`.

## Homebrew

```bash
brew install --cask tamnd/tap/goodread
```

## Scoop

```bash
scoop bucket add tamnd https://github.com/tamnd/scoop-bucket
scoop install goodread
```

## Container image

The multi-arch image is on GHCR:

```bash
docker run --rm ghcr.io/tamnd/goodread:0.1.0 search "the hunger games"
```

## From source

```bash
git clone https://github.com/tamnd/goodread-cli
cd goodread-cli
make build        # produces ./bin/goodread
./bin/goodread version
```

## Requirements

- **Go 1.26 or later** to build. The released binary has no Go requirement.

That is the whole list. The binary is pure Go (CGO_ENABLED=0), so there is no
config file to write, no database to provision, and nothing to link against.

## Checking the install

```bash
goodread version
```

prints the version and exits. Then confirm it can reach Goodreads through an
open endpoint:

```bash
goodread search "the hunger games" -n 3
```

should print a few matching books. If you see them, you are ready for the
[quick start](/getting-started/quick-start/).
