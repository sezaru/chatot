#!/usr/bin/env bash
# Pushes this directory's manifest to the com.sezdm.chatot branch of your
# flathub/flathub fork, from a throwaway clone that is removed afterwards.
#
#   build-aux/flatpak/flathub/push.sh "<commit message, in your own words>"
#
# Opening the pull request, its description and every reply stay manual:
# Flathub's policy forbids AI tools from doing any of that, and the PR
# template asks you to confirm it. Base branch on flathub/flathub: new-pr.
set -euo pipefail

msg=${1:?usage: push.sh "<commit message>"}
here=$(cd "$(dirname "$0")" && pwd)
fork=git@github.com:sezaru/flathub.git
branch=com.sezdm.chatot

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

git clone -q --depth 1 --branch new-pr https://github.com/flathub/flathub.git "$work"
cd "$work"
git checkout -q -B "$branch"
cp -L "$here"/com.sezdm.chatot.yml "$here"/go.mod.yml "$here"/modules.txt .
git add com.sezdm.chatot.yml go.mod.yml modules.txt
git commit -q -m "$msg"
git push -q -f "$fork" "$branch"

echo "pushed $branch to $fork"
echo "open the pull request (base: new-pr):"
echo "  https://github.com/flathub/flathub/compare/new-pr...sezaru:flathub:$branch?expand=1"
