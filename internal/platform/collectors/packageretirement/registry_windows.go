package packageretirement

import (
	"context"
	"debug/pe"
	"encoding/binary"
	"errors"
	"io"
	"runtime"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const uninstallKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`

type registryView struct {
	name string
	flag uint32
}

func Run(ctx context.Context, client, asset int64, publish func(context.Context, []monitoring.DiscoveryEvent) error) error {
	if err := validatePackageRetirementScope(client, asset, publish); err != nil {
		return err
	}
	views, err := applicationRegistryViews()
	if err != nil {
		return err
	}
	probes := []packageProbe{{
		source: "registry",
		read: func(ctx context.Context) (map[packageAddress]packageRecord, error) {
			return readRegistryApplications(ctx, views)
		},
	}}
	return watchPackageProbes(ctx, client, asset, publish, probes)
}

func applicationRegistryViews() ([]registryView, error) {
	if runtime.GOARCH == "386" {
		var process, native uint16
		err := windows.IsWow64Process2(windows.CurrentProcess(), &process, &native)
		if errors.Is(err, windows.ERROR_PROC_NOT_FOUND) {
			var wow64 bool
			err = windows.IsWow64Process(windows.CurrentProcess(), &wow64)
			if err == nil && !wow64 {
				return []registryView{{name: "32"}}, nil
			}
		}
		if err != nil {
			return nil, err
		}
		if native == pe.IMAGE_FILE_MACHINE_I386 {
			return []registryView{{name: "32"}}, nil
		}
	}
	return []registryView{
		{name: "64", flag: registry.WOW64_64KEY},
		{name: "32", flag: registry.WOW64_32KEY},
	}, nil
}

func readRegistryApplications(ctx context.Context, views []registryView) (map[packageAddress]packageRecord, error) {
	var applications []registryApplication
	for _, view := range views {
		var err error
		applications, err = readRegistryView(ctx, view, applications)
		if err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return normalizeRegistryApplications(applications)
}

func readRegistryView(ctx context.Context, view registryView, applications []registryApplication) ([]registryApplication, error) {
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, uninstallKey, registry.READ|view.flag)
	if errors.Is(err, registry.ErrNotExist) {
		return applications, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()

	before, err := root.Stat()
	if err != nil {
		return nil, err
	}
	remaining := packageLimit - len(applications)
	if before.SubKeyCount > uint32(remaining) {
		return nil, errors.New("application registry inventory exceeds limit")
	}
	names, err := root.ReadSubKeyNames(remaining + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		displayName, err := readRegistryDisplayName(root, view, name)
		if err != nil {
			return nil, err
		}
		applications = append(applications, registryApplication{view.name, name, displayName})
	}
	after, err := root.Stat()
	if err != nil {
		return nil, err
	}
	if uint32(len(names)) != before.SubKeyCount || before.SubKeyCount != after.SubKeyCount || !before.ModTime().Equal(after.ModTime()) {
		return nil, errors.New("application registry changed during inventory")
	}
	return applications, nil
}

func readRegistryDisplayName(root registry.Key, view registryView, name string) (string, error) {
	key, err := registry.OpenKey(root, name, registry.QUERY_VALUE|view.flag)
	if err != nil {
		return "", err
	}
	defer key.Close()

	// A 255-byte UTF-8 name fits within 255 UTF-16 code units plus a terminator.
	var buf [512]byte
	size, valueType, err := key.GetValue("DisplayName", buf[:])
	if errors.Is(err, registry.ErrNotExist) || errors.Is(err, windows.ERROR_MORE_DATA) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if (valueType != registry.SZ && valueType != registry.EXPAND_SZ) || size%2 != 0 {
		return "", nil
	}
	var chars [256]uint16
	for i := range size / 2 {
		chars[i] = binary.LittleEndian.Uint16(buf[i*2:])
	}
	displayName := windows.UTF16ToString(chars[:size/2])
	if valueType == registry.EXPAND_SZ {
		return registry.ExpandString(displayName)
	}
	return displayName, nil
}
