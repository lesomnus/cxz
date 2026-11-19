package configeditor

import "golang.org/x/sys/unix"

func exchangeTemplate(staged, live string) error {
	return unix.Renameat2(unix.AT_FDCWD, staged, unix.AT_FDCWD, live, unix.RENAME_EXCHANGE)
}
