package diskretirement

import (
	"regexp"
	"strings"

	"github.com/you/aiceberg_agent/internal/platform/collectors/kernelnetlink"
)

var diskName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.+-]{0,127}$`)

func removedDiskName(raw []byte) string {
	properties := kernelnetlink.DeviceEvent(raw, "block")
	name := properties["DEVNAME"]
	deviceType := properties["DEVTYPE"]
	if properties["ACTION"] != "remove" || (deviceType != "disk" && deviceType != "partition") ||
		!diskName.MatchString(name) || !strings.HasSuffix(properties["DEVPATH"], "/"+name) {
		return ""
	}
	return name
}
