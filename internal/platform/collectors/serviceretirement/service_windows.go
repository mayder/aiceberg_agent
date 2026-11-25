package serviceretirement

import (
	"context"
	"errors"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

type windowsServiceInventory struct{ manager *mgr.Mgr }

func (s windowsServiceInventory) list(context.Context) ([]string, error) {
	return s.manager.ListServices()
}

func (s windowsServiceInventory) missing(_ context.Context, name string) (bool, error) {
	service, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false, err
	}
	handle, err := windows.OpenService(s.manager.Handle, service, windows.SERVICE_QUERY_STATUS)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, windows.CloseServiceHandle(handle)
}

func Run(ctx context.Context, client, asset int64, publish func(context.Context, []monitoring.DiscoveryEvent) error) error {
	if client < 1 || asset < 1 || publish == nil {
		return errors.New("invalid service discovery scope")
	}
	handle, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT|windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return err
	}
	manager := &mgr.Mgr{Handle: handle}
	defer func() { _ = manager.Disconnect() }()
	tracker := retirementTracker{client: client, asset: asset, inventory: windowsServiceInventory{manager}, publish: publish}
	return tracker.watch(ctx)
}
