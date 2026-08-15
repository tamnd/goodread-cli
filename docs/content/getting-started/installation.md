---
title: "Installation"
description: "Install goodread from a release, with go install, from a package manager, or as a container."
weight: 20
---

## Prebuilt binaries

Every [release](https://github.com/tamnd/goodread-cli/releases) carries archives for Linux, macOS, Windows and FreeBSD on amd64 and arm64, plus deb, rpm and apk packages for Linux.
Download, unpack, put `goodread` on your `PATH`, done.
The `checksums.txt` on each release is signed with keyless [cosign](https://docs.sigstore.dev/), and SBOMs ship alongside, if you want to verify before running.

## With Go

```bash
go install github.com/tamnd/goodread-cli/cmd/goodread@latest
```

That puts `goodread` in `$(go env GOPATH)/bin`, which is `~/go/bin` unless you moved it.
Make sure that directory is on your `PATH`.

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
docker run --rm ghcr.io/tamnd/goodread:0.3.0 book 2767052
docker run --rm ghcr.io/tamnd/goodread book 2767052
```

The store and the cache live inside the container unless you mount a directory for them, so a container run that crawls wants a volume:

```bash
docker run --rm -v "$PWD/data:/data" -e GOODREAD_DATA_DIR=/data \
  ghcr.io/tamnd/goodread crawl --seed gr:author/153394 --depth 1
```

## From source

```bash
git clone https://github.com/tamnd/goodread-cli
cd goodread-cli
make build
./bin/goodread version
```

## Requirements

Go 1.26 or later to build.
The released binary has no Go requirement.

That is the whole list.
The binary is pure Go with `CGO_ENABLED=0`, so there is no database to provision and nothing to link against.
A config file is optional and there is no default one to write.

## Checking the install

```bash
goodread version
```

prints the version, the commit and the build date, then exits.
Then confirm it can reach Goodreads and read the rules:

```bash
goodread robots
```

That fetches `robots.txt` and prints every surface with the rule that decides it, which is one request and tells you both that the network works and what the tool is allowed to do.
Then read a real page:

```bash
goodread book 2767052
```

If you see The Hunger Games, you are ready for the [quick start](/getting-started/quick-start/).
