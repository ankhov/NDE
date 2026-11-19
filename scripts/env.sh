#!/usr/bin/env bash
# Workspace-local Go toolchain bootstrap.
# Source this file before any Go command:  source /home/z/my-project/scripts/env.sh
# The Go SDK lives inside the repo (gitignored) so the sandbox stays self-contained
# and no files are written outside /home/z/my-project.
export GOROOT=/home/z/my-project/.sdk/go
export GOPATH=/home/z/my-project/.sdk/gopath
export GOCACHE=/home/z/my-project/.sdk/gocache
export GOMODCACHE=/home/z/my-project/.sdk/gopath/pkg/mod
export GOBIN=/home/z/my-project/.sdk/gopath/bin
export PATH="$GOROOT/bin:$GOBIN:$PATH"
