# VioletTweaks 1.0.3 — Installation & troubleshooting

1. Completely close your old VioletTweaks windows and processes in Task Manager.
2. Double-click **VioletTweaks-English-Fixed.exe**. It opens an isolated Edge or Chrome application window.
3. The entire interface is in English. No account, payments, or subscription are needed.
4. You do not need administrator privileges to view the interface. Some Windows repair/tweak operations explicitly require administrator privileges.
5. For Fortnite Potato Graphics: close Fortnite, open **Potato Graphics**, choose a preset, and click **Apply potato graphics**. Your INI settings are backed up first; **Restore previous graphics** rolls back to the saved settings.

## Fixed startup error

The previous build failed with `Uncaught SyntaxError: Identifier 'top' has already been declared`, due to a global binding named `top` colliding with the browser's `window.top`. This was fixed in 1.0.3. The app now uses a separate browser profile under `%LOCALAPPDATA%\VioletTweaks\BrowserProfile`, which avoids interference from Chrome/Edge translation settings and extensions.

## If it still fails

- Close all older VioletTweaks processes before restarting.
- Open `%LOCALAPPDATA%\VioletTweaks\startup.log` in Notepad for server diagnostics.
- Send the exact displayed error message or a screenshot.

## Important

This app provides 26 reversible registry tweaks, optional application uninstall operations, and reference-only guidance for riskier entries. Actions requiring administrator rights won't silently change Windows. Low-graphics Fortnite presets use ordinary user settings, not protected game files. FPS improvement depends on your computer.
