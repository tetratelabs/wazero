#!/bin/sh -ue
# Copyright 2023 Tetrate
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# This script echos the winget manifest for a wazero Windows Installer (msi).
#
# Ex.
# msi_file=/path/to/wazero.msi
# manifest_path=./manifests/t/Tetrate/wazero/${version}/Tetrate.wazero.yaml
# mkdir -p $(dirname "${manifest_path}")
# packaging/msi/winget_manifest.sh ${version} ${msi_file} > ${manifest_path}
# winget validate --manifest ${manifest_path}

version=${1:-0.0.1}
msi_file=${2:-dist/wazero_dev_windows_amd64.msi}

case $(uname -s) in
CYGWIN* | MSYS* | MINGW*)
  installer_sha256=$(certutil -hashfile "${msi_file}" SHA256 | sed -n 2p)
  product_code=$(powershell -File ./packaging/msi/msi_product_code.ps1 -msi "${msi_file}")
  ;;
*) # notably, this gets rid of the Windows carriage return (\r), which otherwise would mess up the heredoc.
  msiinfo -h export >/dev/null
  # shasum -a 256, not sha256sum as https://github.com/actions/virtual-environments/issues/90
  installer_sha256=$(shasum -a 256 "${msi_file}" | awk '{print toupper($1)}' 2>&-)
  product_code=$(msiinfo export "${msi_file}" Property | sed -n '/ProductCode/s/\r$//p' | cut -f2)
  ;;
esac

cat <<EOF
# yaml-language-server: \$schema=https://aka.ms/winget-manifest.singleton.1.0.0.schema.json
---
PackageIdentifier: Tetrate.wazero
PackageVersion: ${version}
PackageName: wazero
Publisher: Tetrate
Copyright: Copyright 2023 Tetrate
License: Apache 2.0
LicenseUrl: https://github.com/tetratelabs/wazero/blob/main/LICENSE
Moniker: wazero
Commands:
  - wazero
Tags:
  - wazero
  - webassembly
  - wasm
  - tetrate
ShortDescription: wazero runs WebAssembly modules
Description: wazero is a command line utility to run WebAssembly modules. Specifically, this runs .wasm files compiled with WASI or Go, and has zero platform dependencies.
PackageUrl: https://wazero.io
Installers:
  - Architecture: x64
    InstallerUrl: https://github.com/tetratelabs/wazero/releases/download/v${version}/wazero_${version}_windows_amd64.msi
    InstallerSha256: $installer_sha256
    InstallerType: msi
    ProductCode: "${product_code}"
PackageLocale: en-US
ManifestType: singleton
ManifestVersion: 1.0.0
EOF
