#!/bin/sh -ue
#
# This script generates the release notes "wazero" for a specific release tag.
# .github/workflows/release_notes.sh v1.3.0

tag=$1
prior_tag=$(git tag -l 'v*'|sed "/${tag}/,+10d"|tail -1)
if [ -n "${prior_tag}" ]; then
  range="${prior_tag}..${tag}"
else
  range=${tag}
fi

git config log.mailmap true
changelog=$(git log --format='%h %s %aN, %(trailers:key=co-authored-by)' "${range}")

# strip the v off the tag name more shell portable than ${tag:1}
version=$(echo "${tag}" | cut -c2-100) || exit 1
cat <<EOF
wazero ${version} supports X and Y and notably fixes Z

TODO: classify the below into up to 4 major headings and the rest as bulleted items in minor changes
The published release notes should only include the summary statement in this section.

${changelog}

## X packages

Don't forget to cite who was involved and why

## wazero Y

## Minor changes

TODO: don't add trivial things like fixing spaces or non-concerns like build glitches

* Z is now fixed thanks to Yogi Bear

EOF
