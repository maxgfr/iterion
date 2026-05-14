//go:build desktop

package main

/*
#cgo pkg-config: glib-2.0
#include <glib.h>
#include <stdlib.h>
*/
import "C"

import "unsafe"

// setPrgname calls g_set_prgname with the given value. GLib's program
// name is what GTK uses to derive the WM_CLASS class field at window-
// create time (the second value, capitalised: "iterion" → "Iterion").
// Wails's frontend.go does call g_set_prgname when Linux.ProgramName
// is set, but only AFTER NewWindow has already run — too late to
// influence the WM_CLASS the X server stamps. Calling it ourselves at
// the very start of main() works because gtk_init reads g_get_prgname
// when the window is realised, AFTER our call.
//
// Linux-only build because the package only links GLib on Linux.
func setPrgname(name string) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	C.g_set_prgname(cname)
}
