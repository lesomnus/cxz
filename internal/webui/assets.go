package webui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const InstalledAssetsDirectory = "/usr/local/share/cxz/webui"

// AssetsDirectory locates a separately built UI. Explicit configuration wins;
// a local bundle or source build also works without installing system files.
func AssetsDirectory() string {
	if directory := os.Getenv("CXZ_WEB_ASSETS_DIR"); directory != "" {
		return directory
	}
	if executable, err := os.Executable(); err == nil {
		for _, directory := range []string{
			filepath.Join(filepath.Dir(executable), "webui"),
			filepath.Join(filepath.Dir(executable), "..", "webui"), // dist/linux-ARCH/cxz
		} {
			if _, err := os.Stat(filepath.Join(directory, "index.html")); err == nil {
				return directory
			}
		}
	}
	if _, err := os.Stat(filepath.Join(InstalledAssetsDirectory, "index.html")); err == nil {
		return InstalledAssetsDirectory
	}
	directory := filepath.Join("internal", "webui", "assets")
	if _, err := os.Stat(filepath.Join(directory, "index.html")); err == nil {
		return directory
	}
	return InstalledAssetsDirectory
}

// Assets serves files from disk; Go builds do not require the browser bundle.
func Assets(directory string) (fs.FS, error) {
	assets := os.DirFS(directory)
	f, err := assets.Open("index.html")
	if err != nil {
		return nil, fmt.Errorf("web UI assets at %q: %w; build the UI with npm run --prefix ts build or set --assets-dir / CXZ_WEB_ASSETS_DIR", directory, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("web UI index: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("web UI index at %q must be a regular file", directory)
	}
	return assets, nil
}
