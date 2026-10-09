package installer

import (
	"archive/tar"
	_ "embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/lesomnus/cxz/internal/webui"
)

//go:embed web-source.Dockerfile
var webSourceDockerfile []byte

var webRevisionFlag = regexp.MustCompile(`(?:^|\s)main\.buildRevision=([0-9a-f]{40})(?:\s|$)`)
var webRevision = regexp.MustCompile(`^[0-9a-f]{40}$`)

func uiSourceRevision(settings []debug.BuildSetting) string {
	revision, modified := "", ""
	for _, setting := range settings {
		switch setting.Key {
		case "-ldflags":
			if match := webRevisionFlag.FindStringSubmatch(setting.Value); len(match) > 1 {
				return match[1]
			}
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value
		}
	}
	if modified == "false" && webRevision.MatchString(revision) {
		return revision
	}
	return ""
}

// Local bundles allow development/offline installs. Source self-updates only
// replace the executable, so otherwise build UI from its exact recorded revision.
func managerUIBuild() (recipe []byte, directory, revision string, err error) {
	directory = webui.AssetsDirectory()
	if _, statErr := os.Stat(directory); statErr == nil || !os.IsNotExist(statErr) || os.Getenv("CXZ_WEB_ASSETS_DIR") != "" {
		if _, err = webui.Assets(directory); err != nil {
			return nil, "", "", err
		}
		return dockerfile, directory, "", nil
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		revision = uiSourceRevision(info.Settings)
	}
	if revision == "" {
		return nil, "", "", fmt.Errorf("local manager image needs a built UI or a recorded source revision; run npm run --prefix ts build and set CXZ_WEB_ASSETS_DIR, or install --image with a published image")
	}
	recipe = append([]byte(nil), webSourceDockerfile...)
	recipe = append(recipe, []byte(strings.Replace(string(dockerfile), "COPY webui/ /usr/local/share/cxz/webui/", "COPY --from=cxz-web /src/internal/webui/assets/ /usr/local/share/cxz/webui/", 1))...)
	return recipe, "", revision, nil
}

// WalkDir sorts paths. Headers omit build timestamps for a stable image hash.
func writeUIArchive(tw *tar.Writer, directory string) error {
	return fs.WalkDir(os.DirFS(directory), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("web UI asset %q must be a regular file", name)
		}
		f, err := os.Open(filepath.Join(directory, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		defer f.Close()
		if err := tw.WriteHeader(&tar.Header{Name: "webui/" + name, Mode: 0644, Size: info.Size()}); err != nil {
			return err
		}
		_, err = io.Copy(tw, f)
		return err
	})
}
