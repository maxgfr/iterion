//go:build desktop && !linux

package main

// installWindowIcon is a no-op outside Linux. macOS reads the icon
// from the .app bundle's Info.plist+Icon.icns and Windows from the
// .exe resource section, so no runtime intervention is needed there.
// See window_icon_linux.go for the X11/_NET_WM_ICON path that bypasses
// the broken Cinnamon StartupWMClass→.desktop lookup on Mint.
func installWindowIcon() {}
