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

Build the tool into a temporary prefix:

```sh
FWUPD_TOOL="$(./tools/fwupd-macos/build-fwupdtool.sh)"
export DOCKWARDEN_FWUPDTOOL="$FWUPD_TOOL"
```

The script only downloads source and builds fwupdtool.
It does not update firmware or access the dock.

The installed prefix contains fwupd's Dell quirks and plugin data.
Keep the complete prefix. Do not copy only the binary.

Check enumeration without writing firmware:

```sh
"$DOCKWARDEN_FWUPDTOOL" --plugins dell_dock get-devices
./dockwarden status
./dockwarden update
```

Only this command can start the write path:

```sh
./dockwarden update --apply
```

Run it only after reviewing the plan and confirming stable dock power.
Dockwarden passes the verified Dell CAB to upstream fwupdtool.
It does not invoke `sudo`.
