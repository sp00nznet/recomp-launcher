package main

import win "golang.org/x/sys/windows"

func alert(text string) {
	t, _ := win.UTF16PtrFromString(text)
	c, _ := win.UTF16PtrFromString("recomp installer")
	win.MessageBox(0, t, c, win.MB_OK|win.MB_ICONERROR)
}
