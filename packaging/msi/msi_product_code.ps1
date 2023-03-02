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

# msi_product_code.ps1 gets the ProductCode (wix Product.Id) from the given MSI file.
# This must be invoked with Windows.
#
# Ex powershell -File ./packaging/msi/msi_product_code.ps1 -msi /path/to/wazero.msi
#
# See https://docs.microsoft.com/en-us/windows/win32/msi/product-codes
param ([string]$msi)

$windowsInstaller = New-Object -com WindowsInstaller.Installer

# All the below chain from this OpenDatabase
# See https://docs.microsoft.com/en-us/windows/win32/msi/installer-opendatabase
$database = $windowsInstaller.GetType().InvokeMember(
  "OpenDatabase", "InvokeMethod", $Null, $windowsInstaller, @($msi, 0)
)

# We need the ProductCode, which is what wxs Product.Id ends up as:
# See https://docs.microsoft.com/en-us/windows/win32/msi/productcode
$q = "SELECT Value FROM Property WHERE Property = 'ProductCode'"
$View = $database.GetType().InvokeMember("OpenView", "InvokeMethod", $Null, $database, ($q))

try {
  # https://docs.microsoft.com/en-us/windows/win32/msi/view-execute
  $View.GetType().InvokeMember("Execute", "InvokeMethod", $Null, $View, $Null) | Out-Null

  $record = $View.GetType().InvokeMember("Fetch", "InvokeMethod", $Null, $View, $Null)
  $productCode = $record.GetType().InvokeMember("StringData", "GetProperty", $Null, $record, 1)

  Write-Output $productCode
} finally {
  if ($View) {
    $View.GetType().InvokeMember("Close", "InvokeMethod", $Null, $View, $Null) | Out-Null
  }
}
