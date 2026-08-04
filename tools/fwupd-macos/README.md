# macOS fwupdtool build

This directory builds upstream fwupd's standalone Dell Dock tool.
The build uses fwupd commit `61c7cf1873fedd78fa031e8a8829cb3413aaef46`.
That commit reports fwupd version 2.2.1.

The script applies the pinned Darwin patch in `patches/`.
The patch routes HID reports through Apple's IOHIDManager.
This avoids the libusb interface claim used on Linux.

The build needs Apple Silicon Homebrew and these packages:

```sh
brew install gcab glib gnutls json-glib libjcat libusb libxmlb meson ninja pkg-config rust
```

Build the tool into the managed prefix:

```sh
./tools/fwupd-macos/build-fwupdtool.sh
```

The default binary path is:

```text
~/Library/Application Support/dockwarden/fwupd-2.2.1/bin/fwupdtool
```

Pass an absolute prefix as the first argument to build elsewhere. The path must
end in `dockwarden/fwupd-2.2.1`. The script replaces only a prefix that has its
ownership marker or a valid legacy manifest. It installs the complete prefix
and writes `manifest.json`. The manifest records
the fwupd version, source commit, Darwin patch SHA-256, and SHA-256 for every
runtime file under `bin`, `etc/fwupd`, `lib`, and `share/fwupd`. Dockwarden
verifies the complete file set before it accesses the network.

The script creates a fresh source tree at the pinned commit. It applies only the
verified Darwin patch. It uses Jinja2 3.1.6 in a fresh virtual environment. It
then builds fwupdtool and runs the enabled upstream non-hardware test suite. It
excludes `fu-usb-backend-test`. That test expects an empty USB bus and fails
when the Mac has attached USB devices. The script does not update firmware or
access a dock.

The installed prefix contains fwupd's Dell quirks and plugin data.
Keep the complete prefix. Do not copy only the binary.

The binary still links to Homebrew libraries outside the managed prefix.
Dockwarden removes loader overrides from the writer environment. Rebuild and
retest the prefix after Homebrew updates its runtime dependencies.

Check enumeration without writing firmware:

```sh
"$HOME/Library/Application Support/dockwarden/fwupd-2.2.1/bin/fwupdtool" --version --json
"$HOME/Library/Application Support/dockwarden/fwupd-2.2.1/bin/fwupdtool" --plugins dell_dock --json get-devices
./dockwarden status
./dockwarden update
```

`DOCKWARDEN_FWUPDTOOL` is optional. If set, it must contain an absolute path to
a separately managed tool prefix. Dockwarden never uses a writer from `PATH`.

Only this command can start the write path:

```sh
./dockwarden update --apply
```

Run it only after reviewing the plan and confirming stable dock power.
Dockwarden passes the verified Dell CAB to upstream fwupdtool.
It does not invoke `sudo`.
