package main

import (
	"runtime"

	win "golang.org/x/sys/windows"
)

// shellOpen opens p the way a double-click does: a .cmd or .lnk runs (in
// dir, when it isn't ""), a folder or a log opens, a link goes to the
// browser. Windows' own call, not cmd /c start: no cmd to read the % or ^ in
// a name, and an error when it can't.
func shellOpen(p, dir string) error {
	runtime.LockOSThread() // the shell may use COM, which is set up per thread
	defer runtime.UnlockOSThread()
	if win.CoInitializeEx(0, win.COINIT_APARTMENTTHREADED|win.COINIT_DISABLE_OLE1DDE) == nil {
		defer win.CoUninitialize()
	}
	verb, _ := win.UTF16PtrFromString("open")
	file, err := win.UTF16PtrFromString(p)
	if err != nil {
		return err
	}
	var cwd *uint16
	if dir != "" {
		if cwd, err = win.UTF16PtrFromString(dir); err != nil {
			return err
		}
	}
	return win.ShellExecute(0, verb, file, nil, cwd, win.SW_SHOWNORMAL)
}
