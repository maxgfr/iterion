//go:build desktop && !linux

package main

// setPrgname is a no-op outside Linux — macOS / Windows derive the
// app's identity from the .app bundle / exe resource section, not
// GLib's prgname.
func setPrgname(name string) {}
