#!/usr/bin/env bash
set -euo pipefail
S=${S:-$PWD/.scratch}   # working dir for downloads, venv and bundle
P0=$S/phase0; LG=$S/lg/laya-go   # LG = git clone of metalagman/laya-go at v0.2.6
chk() { echo "$2  $1" | sha256sum -c - ; }
cd $P0/dl
# ONNX Runtime 1.29.0
curl -fsSL -o ort.tgz https://github.com/microsoft/onnxruntime/releases/download/v1.29.0/onnxruntime-linux-x64-1.29.0.tgz
chk ort.tgz c3fddc4f139a045b0c4902c57410f0694f1c2fdf9b6939fbe38b1aeae7cd14ba
tar -xzf ort.tgz -C $P0/lib onnxruntime-linux-x64-1.29.0/lib/libonnxruntime.so.1.29.0
chk $P0/lib/onnxruntime-linux-x64-1.29.0/lib/libonnxruntime.so.1.29.0 5715f06d8992ca8eeeddcce43df3a7d38f97d537052126f558e912cb312460ca
# tokenizers
curl -fsSL -o tok.tgz https://github.com/daulet/tokenizers/releases/download/v1.27.0/libtokenizers.linux-amd64.tar.gz
chk tok.tgz 72556cdca798dd4ea7cdaba308e5f0d68a8cb93b67c96edf485b7a0edd7b07f4
tar -xzf tok.tgz -C $P0/lib libtokenizers.a
chk $P0/lib/libtokenizers.a e6862b31745bb7d07980fcee70e49cd3b4318097609180f5d2d3fb394f305d50
# checkpoint at pinned revision, per profile file table
REV=052592a15d198d9ad47da779604259b10b47b7aa
jq -r '.source_files[] | "\(.path) \(.sha256)"' $LG/tools/export/profiles/laya-multilingual-v1.json | while read -r p h; do
  mkdir -p "$P0/src/$(dirname "$p")"
  curl -fsSL -o "$P0/src/$p" "https://huggingface.co/convaiinnovations/laya-multilingual/resolve/$REV/$p"
  chk "$P0/src/$p" "$h"
done
# SDK at pinned revision
SREV=573e5b62696ba441230cd6be71d593331b5d23af
curl -fsSL -o sdk.tgz "https://codeload.github.com/NandhaKishorM/laya/tar.gz/$SREV"
echo "sdk archive sha: $(sha256sum sdk.tgz | cut -d' ' -f1) expected 03931635a92b7609c6c253ac1d4d618ebe4fc54956744db01c027b504fd9c426"
tar -xzf sdk.tgz -C $P0/sdk --strip-components=1
# locked exporter env (online sync once)
cd $LG && UV_CACHE_DIR=$S/uv-cache UV_PROJECT_ENVIRONMENT=$S/export-venv uv sync --project tools/export --frozen
echo DOWNLOAD_DONE
