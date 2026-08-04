# Credits

`dockwarden` is original code by Federico Benedetti.

The project uses these external references and system components:

- [fwupd](https://github.com/fwupd/fwupd): the Dell Dock plugin is the
  protocol reference for HID-I2C reads, writes, version fields, and update
  sequencing. fwupd remains a separate project with its own license.
- [fwupd Dell Dock documentation](https://fwupd.github.io/libfwupdplugin/dell-dock-README.html):
  device identifiers and protocol scope.
- [Dell WD19 firmware driver](https://www.dell.com/support/home/en-us/drivers/driversdetails?driverid=389w0):
  official package metadata and firmware payload source.
- [Dell WD19 Administrator Guide](https://www.dell.com/support/manuals/en-us/dell-wd19-130w-dock/wd19_administrator_guide/updating-the-firmware):
  vendor update guidance.
- [Apple IOKit and CoreFoundation](https://developer.apple.com/documentation/iokit):
  macOS system frameworks used for HID access.
- [libusb](https://libusb.info/): the macOS USB backend used by the standalone
  fwupdtool build.
- [Meson](https://mesonbuild.com/), [Ninja](https://ninja-build.org/), and
  [Rust](https://www.rust-lang.org/): tools used to build upstream fwupdtool.
- [Go](https://go.dev/): the language and standard library.
- `bsdtar`/libarchive: retained by the native HID protocol regression path.

Dell, WD19, fwupd, Apple, macOS, and Go are trademarks or project names owned
by their respective holders. This project is not affiliated with Dell, Apple,
or the fwupd maintainers.

The repository does not include Dell firmware blobs. It downloads official
Dell files at runtime and verifies their published SHA-256 values.
