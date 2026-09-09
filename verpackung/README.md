# Nexora to take with you

*Diese Anleitung gibt es auch [auf Deutsch](LIESMICH.md).*

Nexora is a web application. What is built here is not a second Nexora, it is
a window of its own onto the one already running: the same interface, the same
server, the same state for everyone. A separate store inside the app would
give up the very thing Nexora is for.

What the app adds over a bookmark: a window without an address bar and other
people's tabs, an entry in the start menu or on the home screen, and the
address of the instance settled once instead of in every browser profile
again.

## What gets built

| Target | Result | Builds on |
|---|---|---|
| Windows | `Nexora-windows-x64` with `Nexora.exe` | Linux |
| macOS | `Nexora-macos-arm64.dmg`, `Nexora-macos-x64.dmg` | Linux |
| Linux | `Nexora-linux-x64` with `nexora` | Linux |
| Android | `Nexora-android.apk` | Linux, Android SDK in a container |
| iOS | Xcode project under `mobil/ios` | **macOS with Xcode only** |

## Desktop (Windows, macOS, Linux)

```
cd schreibtisch && npm install
node bauen.mjs
```

`bauen.mjs` fetches the prebuilt Electron release per platform, puts the
application into `resources/app` and renames the binary. A packer does the
same three things — but `electron-packager` demands Wine on Linux as soon as a
Windows target is to carry metadata, and metadata already comes from
`package.json`. A gigabyte of Wine for the name in a file dialog is not worth
it.

The DMG is made with `genisoimage` as an Apple ISO. macOS mounts it like any
other image.

## Android

```
cd mobil && npm install
npx cap sync android
```

The build runs in the container that carries the Android SDK, on the Docker
host:

```
docker run --rm -v <path>/mobil:/build -v nexora-gradle-cache:/root/.gradle \
  -w /build/android hecht-android:2 ./gradlew --no-daemon assembleDebug
```

The result is at `android/app/build/outputs/apk/debug/app-debug.apk`.

Note that this is a *debug* build. Only that one carries
`usesCleartextTraffic` automatically, and without it Android 9 and newer
refuse plain HTTP without a word — the app would show a blank page. A release
build needs the flag stated explicitly, or the instance needs TLS.

## iOS

The project under `mobil/ios` is complete, but an `.ipa` is only produced on a
Mac with Xcode, and installing it needs an Apple developer account. Without
one the way in is the browser: open Nexora in Safari, Share, "Add to Home
Screen". That gives an icon without an address bar — practically the same
thing.

## Address of the instance

The default is `http://10.0.2.43:3000`.

* Desktop: in the menu under *Nexora → Adresse ändern…*; it is remembered in
  the application data. Alternatively the environment variable
  `NEXORA_ADRESSE`.
* Android and iOS: in `capacitor.config.json`, then `npx cap sync`.

## What the window is allowed to do

The wrapper hands only `http` and `https` to the operating system.
`shell.openExternal` passes an address to the OS, and that opens `file:`,
`smb:` or `ms-msdt:` just as happily — a page could otherwise start programs
on the machine through a `window.open`.

The window itself also stays with the origin of its instance. Without that a
link in the content could send the window to a foreign page, and that page
would look like Nexora, password field included. Links leading outside go to
the browser instead.

## Signatures

None of this is signed.

* Windows shows SmartScreen on first start: *More info → Run anyway*.
* macOS refuses a double click: right-click → *Open* once, then it remembers.
* Android asks for "install from unknown sources" for the app you open the APK
  from.

Changing that costs money and time: a Windows certificate per year, an Apple
developer account per year. For home use on your own network it is not worth
it.
