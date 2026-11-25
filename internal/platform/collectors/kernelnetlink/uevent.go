package kernelnetlink

import (
	"bytes"
	"path"
	"strings"
)

// DeviceEvent accepts a complete, non-synthetic uevent datagram from Receive.
func DeviceEvent(raw []byte, subsystem string) map[string]string {
	if len(raw) < 1 || len(raw) > 8192 || raw[len(raw)-1] != 0 {
		return nil
	}
	fields := bytes.Split(bytes.TrimRight(raw, "\x00"), []byte{0})
	if len(fields) < 4 || len(fields) > 64 {
		return nil
	}
	properties := make(map[string]string)
	for _, field := range fields[1:] {
		key, value, ok := strings.Cut(string(field), "=")
		if !ok || key == "" || strings.HasPrefix(key, "SYNTH_") {
			return nil
		}
		if _, repeated := properties[key]; repeated {
			return nil
		}
		properties[key] = value
	}
	devicePath := properties["DEVPATH"]
	action := properties["ACTION"]
	if action == "" || properties["SUBSYSTEM"] != subsystem ||
		!strings.HasPrefix(devicePath, "/devices/") || path.Clean(devicePath) != devicePath ||
		string(fields[0]) != action+"@"+devicePath {
		return nil
	}
	return properties
}
