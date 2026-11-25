package logdiscovery

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	eventLogAPI         = windows.NewLazySystemDLL("wevtapi.dll")
	openEventChannel    = eventLogAPI.NewProc("EvtOpenChannelConfig")
	closeEventLogHandle = eventLogAPI.NewProc("EvtClose")
)

func probeEventChannel(channel string) (channelPresence, error) {
	if err := openEventChannel.Find(); err != nil {
		return channelUnknown, err
	}
	if err := closeEventLogHandle.Find(); err != nil {
		return channelUnknown, err
	}
	path, err := windows.UTF16PtrFromString(channel)
	if err != nil {
		return channelUnknown, err
	}
	handle, _, callErr := openEventChannel.Call(0, uintptr(unsafe.Pointer(path)), 0)
	if handle == 0 {
		if errors.Is(callErr, windows.ERROR_EVT_CHANNEL_NOT_FOUND) {
			return channelMissing, nil
		}
		return channelUnknown, callErr
	}
	if closed, _, err := closeEventLogHandle.Call(handle); closed == 0 {
		return channelUnknown, err
	}
	return channelRegistered, nil
}
