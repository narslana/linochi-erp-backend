#!/usr/bin/env bash
# Works out what a build of HEAD is called, for CI and for `make image`.
#
#   VERSION  0.1.0 when HEAD carries the tag v0.1.0, otherwise the newest tag with
#            -dev after it (0.1.0-dev), or 0.0.0-dev when there is no tag yet.
#   COMMIT   the short commit, 7 characters (what the menu shows).
#   TAG      the image tag: <version>-<sha8>. Never reused, so Argo CD can see that
#            it moved (a moving tag would show Synced while the old image runs).
#
# Prints KEY=VALUE lines, ready for $GITHUB_OUTPUT. Needs the tags: in CI check
# out with fetch-depth 0.
set -euo pipefail

if tag=$(git describe --exact-match --tags --match 'v[0-9]*' HEAD 2>/dev/null); then
  version="${tag#v}"
elif tag=$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null); then
  version="${tag#v}-dev"
else
  version="0.0.0-dev"
fi
commit=$(git rev-parse --short=7 HEAD)
sha8=$(git rev-parse --short=8 HEAD)

echo "VERSION=${version}"
echo "COMMIT=${commit}"
echo "TAG=${version}-${sha8}"
