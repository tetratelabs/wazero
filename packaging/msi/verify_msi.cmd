:: Copyright 2023 Tetrate
::
:: Licensed under the Apache License, Version 2.0 (the "License");
:: you may not use this file except in compliance with the License.
:: You may obtain a copy of the License at
::
::     http://www.apache.org/licenses/LICENSE-2.0
::
:: Unless required by applicable law or agreed to in writing, software
:: distributed under the License is distributed on an "AS IS" BASIS,
:: WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
:: See the License for the specific language governing permissions and
:: limitations under the License.

:: verify_msi is written in cmd because msiexec doesn't agree with git-bash
:: See https://github.com/git-for-windows/git/issues/2526
@echo off
if not defined MSI_FILE set MSI_FILE=dist\wazero_dev_windows_amd64.msi
echo installing %MSI_FILE%
msiexec /i %MSI_FILE% /qn || exit /b 1

:: Use chocolatey tool to refresh the current PATH without exiting the shell
call RefreshEnv.cmd

echo ensuring wazero was installed
wazero version || exit /b 2

echo uninstalling wazero
msiexec /x %MSI_FILE% /qn || exit /b 3

echo ensuring wazero was uninstalled
wazero version && exit /b 4
:: success!
exit /b 0
