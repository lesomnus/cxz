package download

import "golang.org/x/sys/windows"

func Directory() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_Downloads, windows.KF_FLAG_DONT_VERIFY)
}
