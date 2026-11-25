package diskretirement

import (
	"strings"
	"testing"
)

func TestKernelRemovalUsesTheDiskstatsName(t *testing.T) {
	wire := "remove@/devices/pci0000/host/block/sda/sda2\x00ACTION=remove\x00DEVPATH=/devices/pci0000/host/block/sda/sda2\x00SUBSYSTEM=block\x00DEVNAME=sda2\x00DEVTYPE=partition\x00MAJOR=8\x00MINOR=2\x00"
	if got := removedDiskName([]byte(wire)); got != "sda2" {
		t.Fatalf("identity %q does not match disk I/O identity", got)
	}
	for name, candidate := range map[string]string{
		"truncated":       strings.TrimSuffix(wire, "\x00"),
		"synthetic":       wire + "SYNTH_UUID=0\x00",
		"change":          strings.ReplaceAll(wire, "remove", "change"),
		"different name":  strings.Replace(wire, "DEVNAME=sda2", "DEVNAME=sdb2", 1),
		"path traversal":  strings.Replace(wire, "DEVNAME=sda2", "DEVNAME=../sda2", 1),
		"duplicated name": wire + "DEVNAME=sda2\x00",
		"not block":       strings.Replace(wire, "SUBSYSTEM=block", "SUBSYSTEM=net", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if got := removedDiskName([]byte(candidate)); got != "" {
				t.Fatalf("unsupported notification escaped validation: %q", got)
			}
		})
	}
}
