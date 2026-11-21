# Verification record — 2026-11-20 (0.1.1: shell cold start)

## Why

On the vivo X Fold5 (AndroMeld 2.0.0 / 26101400) the Agent was absent from 2026-11-19 13:11 to 2026-11-20 22:14 PST while 0.1.0 was in protect mode and logged ~60 `daemon_bootstrap_requested` events:

- 0.1.0 cold start ran `app_process` as root with only CLASSPATH/PATH/ANDROID_ROOT/ANDROID_DATA. Without the ART/i18n/tzdata roots `app_process` exits silently before main; logcat shows no `Calling main entry` for any of those launches.
- Official starts (Mac / App, via adbd) are uid 2000. A previously root-run Agent had left `clients.json` and `web_service.json` owned by root, so every shell start crashed with `chmod failed: EPERM` (50 times). Handing those two files back to shell restored the Agent immediately.

## Change

- Cold start: uid/gid 2000 + adbd shell groups; runtime variables copied from zygote by whitelist; refuses to launch when a required one is missing.
- Accepted live identity: shell or root (uid == euid).
- Right before launch with no Agent alive: root-owned regular `*.json` in the Agent directory → 2000:2000. Keys and the directory are never chowned; if they are not shell-owned the launch is skipped.

## Checked

- 36 Go unit tests pass (30 existing + 6 new: launch env, missing runtime roots, PATH default, identity, shell groups, ownership selection); `go vet` and `gofmt` clean. Run on macOS; Android binaries cross-built with Go 1.27.4 `GOOS=android GOARCH=arm64 CGO_ENABLED=0`.
- Real device probe (`TestDeviceProbeShellLaunch`, root, vivo): `id` under the same credentials printed `uid=2000(shell) gid=2000(shell)` with exactly the adb-shell groups including `3003(inet)`; `app_process … com.android.commands.am.Am help` reached main as shell with the zygote-derived environment. SELinux context stays `u:r:ksu:s0`.
- On device, the stripped 0.1.0 environment reproduced the silent exit with `am help`; adding the three APEX roots fixed it.

## Still unverified for 0.1.1

- An actual cold start of AndroMeld's `DeviceAgentMain` by Guardian after a real Agent absence (the running Agent was not killed to test it).
- Whether anything in AndroMeld behaves differently under `ksu` vs `shell` SELinux context.

# Verification record — 2026-11-05

## Build

- Source: `src/` (Go standard library only), Go 1.27.1 android/arm64, CGO_ENABLED=0, PIE, netgo/osusergo.
- Module: `dist/AndroMeld-Guardian-v0.1.0-arm64.zip`, 1,566,275 bytes.
- ZIP SHA256: `5a61fab02ab4f0fe4cb5e4f1849075a880650cccb6f4947aae89b6b6979a7664`.
- ELF: arm64, `/system/bin/linker64`, no NEEDED/RPATH/RUNPATH entries. Does not depend on a Termux shared library/interpreter.
- 30 Go unit tests passed, including policy, bounds, strict schema/privacy, owned socket tuple, consent revocation, dispatch-time stop gate and corrupt-history fail-closed behavior.
- 12 Node VM WebUI checks passed in Alpine. Native KernelSU exec signature independently compared against official npm `kernelsu` 3.3.0 source; package was read, not installed.
- Chromium file-origin test passed for actual `<dialog>`/CSP/callback contract with mock shell only: exactly one policy-protect command after confirmation. Not a real KernelSU-manager UI test.
- Browser screenshot reviewed; no remote/CDN resources or HTTP listener used by the module.

## Device behavior

- Installed AndroMeld is **2.0.0 build26101400**; old Skill build26092917 was NOT blindly reused.
- New APK code inspected for admin UID gate, FCM wake handlers, Web/MacLink foreground-service lifecycle, idle renewal and daemon single-instance startup.
- One real local root `open_web_service` event succeeded: gateway enabled, app prefs true and foreground service present.
- One real local root `open_mac_link` event succeeded: mac.link active true; pairing remained false; original daemon unchanged.
- These are local wake-path tests, NOT verification of cloud FCM delivery or a remote computer connection.
- New native compiled program queried status successfully from both workspace and staged KernelSU path. These runs were observation-only and did not restart/kill daemon.
- No intentional daemon death, process kill, SELinux change, battery/global policy change, account/credential edit or phone reboot performed.

## Independent review

Initial review found three issues, all corrected before installation:
1. Non-hot/staged upgrades also require reauthorization (`reauthorize` flag).
2. Stop/disable is rechecked at actual broadcast/process-start dispatch; stop stays persistent after a timeout.
3. Bad/missing history cannot silently become an empty trusted rate-limit history while protection remains enabled.

Follow-up independent read-only review found these fixes satisfy the requested boundaries, without a directly introduced blocker. No agent executed phone actions during review.

## Historical first-installation snapshot — 2026-11-05

This is a dated validation record, NOT a live statement about a later installation. Always recheck active/staged module paths, update flag, worker, mode and timestamp.

- Used the official KernelSU installer (`ksud module install`), version 3.3.1 / uapi5.
- Installer successfully staged this module in `/data/adb/modules_update/andromeld_guardian`; it did **not** hot-apply it.
- KernelSU list: enabled=true, update=true, web=false **before the required phone reboot**. The active module directory is only a placeholder; do not manually replace it to fake hot-install.
- All **10 runtime files** in staging match the source/binary manifest. `customize.sh` is installer-only and KernelSU removes it after executing.
- No guardian worker is currently running. Staged `tick` returned observation mode and responsive unchanged daemon, with access entries idle/closed.
- User must reboot when convenient, then open KernelSU → AndroMeld Guardian → WebUI and explicitly enable protection.
- Install ZIP also copied to `/sdcard/Download/AndroMeld-Guardian-v0.1.0-arm64.zip`.

## Still unverified

- Module boot startup after actual phone reboot; KernelSU WebView rendering on device.
- Locked-screen/deep-sleep long-term behavior and battery impact.
- Cold daemon recovery after actual absence; it was not forced merely to test.
- Remote computer reconnect/ICE success under a real cross-network interruption.

Do not describe this as 'already keeping everything online' until the user activates it and those checks are completed. Deep sleep may delay checks; a live unresponsive daemon is deliberately NOT killed.

## Development transport cleanup

Temporary source-only loopback transfers were one-shot and checked closed. They are not part of the module. No standing HTTP service, unrelated module changes or Aether Extension were installed. Repository publishing and the separate Skill documentation update are later maintenance work, not additional evidence of runtime/cold-start reliability.
