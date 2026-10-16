fastlane documentation
----

# Installation

Make sure you have the latest version of the Xcode command line tools installed:

```sh
xcode-select --install
```

For _fastlane_ installation instructions, see [Installing _fastlane_](https://docs.fastlane.tools/#installing-fastlane)

# Available Actions

## iOS

### ios prebuild

```sh
[bundle exec] fastlane ios prebuild
```

Regenerate ios/ from the app config (expo prebuild --clean)

### ios build

```sh
[bundle exec] fastlane ios build
```

Build a signed Release .ipa for App Store Connect

### ios upload

```sh
[bundle exec] fastlane ios upload
```

Upload the last built .ipa to TestFlight

### ios beta

```sh
[bundle exec] fastlane ios beta
```

Build + upload to TestFlight in one go

### ios screenshots

```sh
[bundle exec] fastlane ios screenshots
```

Capture App Store screenshots on the simulator (screenshots/run.sh)

### ios upload_screenshots

```sh
[bundle exec] fastlane ios upload_screenshots
```

Upload fastlane/screenshots/ to the App Store listing (replaces existing ones)

----

This README.md is auto-generated and will be re-generated every time [_fastlane_](https://fastlane.tools) is run.

More information about _fastlane_ can be found on [fastlane.tools](https://fastlane.tools).

The documentation of _fastlane_ can be found on [docs.fastlane.tools](https://docs.fastlane.tools).
