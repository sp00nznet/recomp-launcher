package main

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// regValue reads a string value, HKLM\key\value or HKCU\key\value; "" when it isn't there.
func regValue(p string) string {
	i := strings.LastIndex(p, `\`)
	if i < 0 {
		return ""
	}
	root, key, _ := strings.Cut(p[:i], `\`)
	k, ok := map[string]registry.Key{"HKLM": registry.LOCAL_MACHINE, "HKCU": registry.CURRENT_USER}[root]
	if !ok {
		return ""
	}
	h, err := registry.OpenKey(k, key, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer h.Close()
	v, _, err := h.GetStringValue(p[i+1:])
	if err != nil {
		return ""
	}
	return v
}
