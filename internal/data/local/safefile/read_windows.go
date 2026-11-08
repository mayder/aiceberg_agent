//go:build windows

package safefile

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
)

func readFile(path string, limit int) ([]byte, error) {
	pins, err := pinDirectoriesMode(filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, pin := range pins {
			_ = windows.CloseHandle(pin)
		}
	}()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	err = windows.GetFileInformationByHandle(handle, &info)
	if err != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		_ = windows.CloseHandle(handle)
		if err == nil {
			err = ErrInvalidFile
		}
		return nil, err
	}
	return readBounded(os.NewFile(uintptr(handle), filepath.Base(path)), limit)
}
