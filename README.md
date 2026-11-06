# VioletTweaks — Windows performance toolkit

A free, local Windows 10/11 desktop optimizer inspired by the screenshots. No payment, login, telemetry, or points. Source code included.

## Run

Double-click **VioletTweaks.exe**. It launches an Edge or Chrome app-mode window, or your default browser if neither is found. **Windows 10/11, 64-bit**. The hidden local server exits automatically after about six minutes without an open client window. It only binds to `127.0.0.1` on a random port.

Use *Run as administrator* only for advanced changes and repairs explicitly marked as requiring elevation. Regular per-user options work without admin.

## Build from source

Install Go 1.22 or newer. From this directory:

```shell
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags="-s -w -H=windowsgui" -o VioletTweaks.exe .
```

Or run `build-windows.bat` on Windows with Go installed.

## Safety and implementation notes

- Supported toggles actually modify specific Windows registry entries. Original values are backed up under `%LOCALAPPDATA%\VioletTweaks\state.json` and restored when toggled off. The app does not run blanket debloat scripts or edit Windows Defender/Windows Update.
- Dangerous or unreliable tweaks are **Preview only**; clicking them shows and logs the limitation without changing your computer. They do not pretend to boost FPS. The interface explicitly distinguishes these from working tweaks.
- Windows repair actions use standard built-in tools, provide logs, and may require administrator privileges or a reboot. Application removals are limited to a list of optional per-user Store packages and must be individually confirmed. Restore by reinstalling from Microsoft Store; packages aren't automatically restorable.
- Game Booster terminates **only user-selected processes** from its displayed list on explicit confirmation. It never terminates system processes or Fortnite itself and does not install a background killer.
- Fortnite Potato Graphics updates the *user's normal GameUserSettings.ini* only with the game closed, makes a timestamped backup, and provides restoration. Changes might be overridden by the game; select Performance Rendering Mode inside Fortnite. It doesn't change any protected game assets or anti-cheat components. Higher FPS is never guaranteed.
- A restore point is best practice before changing advanced settings. System Restore needs to be enabled and generally requires administrator rights.
- CPU and RAM are measured via Windows APIs. GPU telemetry uses `nvidia-smi` if NVIDIA utilities are present; otherwise displays “N/A” rather than inventing values. Startup list manages the current user's `Run` registry entries; other startup mechanisms may not appear.
- Locally bound web UI requires Edge/Chrome for app-style window. The .exe contains the front end and service; no internet needed, apart from launching external websites or optional package installation.

## Privacy

No online analytics. Every UI request is made to the local companion service, which uses a random token to protect changes. No remote listening address, accounts, subscriptions, or payments.

## Startup fix / English-only interface (1.0.3)

The browser startup error `Identifier 'top' has already been declared` was reproduced and fixed by renaming the global page header function. A real Chromium navigation smoke test now covers all main sections. The dashboard renders from embedded data before hardware telemetry. Every API request has a timeout, and the watchdog shows errors rather than an endless spinner. The app uses a separate Edge/Chrome profile to avoid browser extensions and automatic Polish translation, and marks the HTML as English and `translate=no`. Diagnostics: `%LOCALAPPDATA%\VioletTweaks\startup.log`. Close any older VioletTweaks processes before launching the new EXE.
