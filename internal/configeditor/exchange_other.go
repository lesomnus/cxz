//go:build !linux

package configeditor

import "errors"

func exchangeTemplate(_, _ string) error {
	return errors.New("replacing an existing template requires Linux directory exchange support")
}
