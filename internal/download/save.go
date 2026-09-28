// Package download saves streamed container files on the frontend machine.
package download

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

func filename(source string) string {
	name := path.Base(source)
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(name, " .")
	if name == "" || name == "." || name == ".." {
		name = "download"
	}
	stem := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9' {
		name = "_" + name
	}
	return name
}

// Save publishes only after transfer succeeds and never overwrites an existing
// destination. The exclusive final create also works on filesystems without links.
func Save(ctx context.Context, dir, source string, receive func(io.Writer) error) (string, int64, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", 0, err
	}
	temp, err := os.CreateTemp(dir, ".cxz-download-*")
	if err != nil {
		return "", 0, err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	if err = receive(temp); err != nil {
		return "", 0, err
	}
	if err = ctx.Err(); err != nil {
		return "", 0, err
	}
	if _, err = temp.Seek(0, io.SeekStart); err != nil {
		return "", 0, err
	}
	name := filename(source)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 0; i < 10000; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		target := filepath.Join(dir, candidate)
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", 0, err
		}
		n, copyErr := io.Copy(out, temp)
		if copyErr == nil {
			copyErr = out.Sync()
		}
		closeErr := out.Close()
		if copyErr == nil {
			copyErr = closeErr
		}
		if copyErr == nil {
			copyErr = ctx.Err()
		}
		if copyErr != nil {
			os.Remove(target)
			return "", 0, copyErr
		}
		return target, n, nil
	}
	return "", 0, fmt.Errorf("too many files named %s", name)
}
