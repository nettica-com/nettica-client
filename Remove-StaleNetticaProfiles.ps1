<#
.SYNOPSIS
    Removes stale Windows network profiles left behind by Nettica WireGuard tunnels.

.DESCRIPTION
    WireGuard for Windows derives each tunnel adapter's GUID from the tunnel config,
    including every peer. Whenever the peer list changes, the adapter comes back with a
    new GUID and Windows creates a new network profile ("nettica 242", "relay 75", ...).
    Old profiles are never removed.

    This script deletes profiles that:
      - were created by a virtual adapter (NameType 53, which WireGuard adapters report),
      - belong to a Nettica network name (or to any virtual adapter, with -AllVirtual), and
      - are not connected right now.

    It also removes the network signatures that point at those profiles. Connected
    profiles are never touched. By default the script only reports what it would do;
    pass -Apply to make changes. A .reg backup of the NetworkList key is written first.

.PARAMETER Apply
    Delete the stale profiles. Without this switch the script only reports.

.PARAMETER Names
    Extra network names to treat as Nettica networks, in addition to the ones found
    from tunnel configs, tunnel services and live WireGuard adapters.

.PARAMETER AllVirtual
    Target every disconnected virtual-adapter profile, not just Nettica network names.
    Use this to also clear networks whose configs are already gone.

.PARAMETER RenameActive
    Rename connected Nettica profiles back to the bare network name
    ("nettica 242" becomes "nettica").

.PARAMETER BackupPath
    Where to write the registry backup. Defaults to C:\ProgramData\Nettica\.

.EXAMPLE
    .\Remove-StaleNetticaProfiles.ps1
    Report only.

.EXAMPLE
    .\Remove-StaleNetticaProfiles.ps1 -Apply -RenameActive
    Delete stale profiles, then rename the connected ones.

.NOTES
    Restore with: reg import <backup file>, then reboot.
#>
#Requires -RunAsAdministrator
[CmdletBinding()]
param(
    [switch]$Apply,
    [string[]]$Names = @(),
    [switch]$AllVirtual,
    [switch]$RenameActive,
    [string]$BackupPath = (Join-Path $env:ProgramData ('Nettica\NetworkList-backup-{0:yyyyMMdd-HHmmss}.reg' -f (Get-Date)))
)

$ErrorActionPreference = 'Stop'

$NetworkListKey  = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\NetworkList'
$ProfilesKey     = "$NetworkListKey\Profiles"
$SignatureKeys   = "$NetworkListKey\Signatures\Unmanaged", "$NetworkListKey\Signatures\Managed"
$VirtualNameType = 53   # IF_TYPE_PROP_VIRTUAL, reported by WireGuard and Wintun adapters
$NlmConnected    = 1    # NLM_ENUM_NETWORK_CONNECTED

function New-NameSet {
    , [System.Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
}

# DateCreated / DateLastConnected are stored as a 16-byte SYSTEMTIME.
function ConvertFrom-SystemTime([byte[]]$Bytes) {
    if (-not $Bytes -or $Bytes.Length -lt 16) { return $null }
    $f = 0..7 | ForEach-Object { [BitConverter]::ToUInt16($Bytes, $_ * 2) }
    try { [datetime]::new($f[0], $f[1], $f[3], $f[4], $f[5], $f[6]) } catch { $null }
}

# --- Nettica network names ------------------------------------------------------------

$netticaNames = New-NameSet
$Names | ForEach-Object { [void]$netticaNames.Add($_) }

# Tunnel configs written by nettica-client, including networks that are no longer active.
$confDir = Join-Path $env:ProgramData 'Nettica\Wireguard'
if (Test-Path $confDir) {
    Get-ChildItem $confDir -Filter *.conf | ForEach-Object { [void]$netticaNames.Add($_.BaseName) }
}

# Tunnel services: WireGuardTunnel$<net> (nettica-client) and net-<net> (Nettica app).
Get-Service -Name 'WireGuardTunnel$*', 'net-*' -ErrorAction SilentlyContinue | ForEach-Object {
    [void]$netticaNames.Add(($_.Name -replace '^(WireGuardTunnel\$|net-)', ''))
}

# Live WireGuard adapters.
Get-NetAdapter -IncludeHidden | Where-Object InterfaceDescription -like 'WireGuard Tunnel*' | ForEach-Object {
    [void]$netticaNames.Add($_.Name)
}

# --- Profiles in use right now ----------------------------------------------------------

# Profile names are unique, so matching by name is enough to protect live profiles.
$connected = New-NameSet
Get-NetConnectionProfile -ErrorAction SilentlyContinue | ForEach-Object { [void]$connected.Add($_.Name) }

$nlm = [Activator]::CreateInstance([Type]::GetTypeFromCLSID([Guid]'DCB00C01-570F-4A9B-8D69-199FDBA5723B'))
@($nlm.GetNetworks($NlmConnected)) | ForEach-Object { [void]$connected.Add($_.GetName()) }

# --- Classify every profile ---------------------------------------------------------------

$profiles = foreach ($key in Get-ChildItem $ProfilesKey) {
    $p = Get-ItemProperty $key.PSPath
    $base = if ($p.Description) { $p.Description } else { $p.ProfileName -replace '\s+\d+$', '' }
    $isVirtual = $p.NameType -eq $VirtualNameType
    $isNettica = $netticaNames.Contains($base)
    $isConnected = $connected.Contains($p.ProfileName)

    [pscustomobject]@{
        Guid          = $key.PSChildName
        Name          = $p.ProfileName
        Base          = $base
        NameType      = $p.NameType
        LastConnected = ConvertFrom-SystemTime $p.DateLastConnected
        Connected     = $isConnected
        Nettica       = $isNettica
        Stale         = $isVirtual -and -not $isConnected -and ($isNettica -or $AllVirtual)
        Path          = $key.PSPath
    }
}

$stale = @($profiles | Where-Object Stale)

Write-Host ''
Write-Host "Network profiles on this machine: $($profiles.Count)"
Write-Host "Nettica network names found:      $($netticaNames.Count)"
Write-Host "Connected profiles (kept):        $(@($profiles | Where-Object Connected).Count)"
Write-Host "Stale profiles to remove:         $($stale.Count)"
Write-Host ''

if ($stale.Count -gt 0) {
    Write-Host 'Stale profiles by network:'
    $stale | Group-Object Base | Sort-Object Count -Descending | ForEach-Object {
        $last = ($_.Group | Measure-Object LastConnected -Maximum).Maximum
        [pscustomobject]@{ Network = $_.Name; Profiles = $_.Count; LastConnected = $last }
    } | Format-Table -AutoSize | Out-Host
}

if (-not $AllVirtual) {
    $otherVirtual = @($profiles | Where-Object { $_.NameType -eq $VirtualNameType -and -not $_.Connected -and -not $_.Nettica })
    if ($otherVirtual.Count -gt 0) {
        Write-Host "Other disconnected virtual-adapter profiles, NOT targeted ($($otherVirtual.Count)). Add -AllVirtual or -Names to include them:"
        $otherVirtual | Group-Object Base | Sort-Object Count -Descending |
            Select-Object @{ n = 'Network'; e = { $_.Name } }, @{ n = 'Profiles'; e = { $_.Count } } |
            Format-Table -AutoSize | Out-Host
    }
}

$skippedType = @($profiles | Where-Object { $_.Nettica -and -not $_.Connected -and $_.NameType -ne $VirtualNameType })
if ($skippedType.Count -gt 0) {
    Write-Host "Skipped $($skippedType.Count) profile(s) with a Nettica name but a non-virtual adapter type:"
    $skippedType | Select-Object Name, NameType, LastConnected | Format-Table -AutoSize | Out-Host
}

# --- Remove ---------------------------------------------------------------------------------

if ($stale.Count -gt 0) {
    if (-not $Apply) {
        Write-Host 'Report only. Run again with -Apply to remove these profiles.' -ForegroundColor Yellow
    } else {
        $regPath = $NetworkListKey -replace '^(HK\w+):', '$1'
        & reg.exe export $regPath $BackupPath /y | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Registry backup failed (reg.exe exit code $LASTEXITCODE). Nothing was changed." }
        Write-Host "Backup written to $BackupPath"

        $staleGuids = New-NameSet
        $stale | ForEach-Object { [void]$staleGuids.Add($_.Guid) }

        $signaturesRemoved = 0
        foreach ($sigKey in $SignatureKeys | Where-Object { Test-Path $_ }) {
            foreach ($sig in Get-ChildItem $sigKey) {
                $profileGuid = (Get-ItemProperty $sig.PSPath -Name ProfileGuid -ErrorAction SilentlyContinue).ProfileGuid
                if ($profileGuid -and $staleGuids.Contains($profileGuid)) {
                    Remove-Item $sig.PSPath -Recurse
                    $signaturesRemoved++
                }
            }
        }

        foreach ($p in $stale) { Remove-Item $p.Path -Recurse }

        Write-Host "Removed $($stale.Count) profiles and $signaturesRemoved signatures." -ForegroundColor Green
    }
}

# --- Rename connected profiles --------------------------------------------------------------

if ($RenameActive) {
    # Names still in use once the stale profiles are gone (in report mode, as if they were).
    $remainingNames = New-NameSet
    $profiles | Where-Object { -not $_.Stale } | ForEach-Object { [void]$remainingNames.Add($_.Name) }

    foreach ($net in @($nlm.GetNetworks($NlmConnected))) {
        $current = $net.GetName()
        $base = $net.GetDescription()
        if (-not $netticaNames.Contains($base) -or $current -eq $base) { continue }

        if ($remainingNames.Contains($base)) {
            Write-Warning "Can't rename '$current' to '$base': another profile already uses that name."
            continue
        }

        if (-not $Apply) {
            Write-Host "Would rename '$current' to '$base'."
            continue
        }
        try {
            $net.SetName($base)
            Write-Host "Renamed '$current' to '$base'."
        } catch {
            Write-Warning "Couldn't rename '$current' to '$base': $($_.Exception.Message). Reboot, then run again with -Apply -RenameActive."
        }
    }
}

if ($Apply -and $stale.Count -gt 0) {
    Write-Host ''
    Write-Host 'Reboot so Windows reloads the network list. Windows Security keeps showing the old list until then.' -ForegroundColor Yellow
}
