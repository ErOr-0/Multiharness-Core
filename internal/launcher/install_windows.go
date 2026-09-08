package launcher

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func installPath(dir string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	value, kind, err := key.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return err
	}
	for _, entry := range strings.Split(value, ";") {
		if strings.EqualFold(strings.TrimRight(entry, "\\/"), dir) {
			return nil
		}
	}
	value = strings.TrimRight(value, ";") + ";" + dir
	if kind == registry.EXPAND_SZ {
		err = key.SetExpandStringValue("Path", value)
	} else {
		err = key.SetStringValue("Path", value)
	}
	if err != nil {
		return err
	}
	// Notify Explorer so subsequently opened terminals inherit the user PATH.
	label, _ := windows.UTF16PtrFromString("Environment")
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	_, _, _ = proc.Call(0xffff, 0x1a, 0, uintptr(unsafe.Pointer(label)), 2, 5000, 0)
	return nil
}
