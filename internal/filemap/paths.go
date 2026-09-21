package filemap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ShareVariable = "${CXZ_SHARE_DIR}"

// ShareDir is the user-managed source directory shared by this installation's
// projects. It is not a container path and is resolved before publication.
func ShareDir(root string) string { return filepath.Join(root, "share") }

func SourcePath(root, src string) (string, error) {
	if strings.Contains(src, ShareVariable) {
		if root == "" {
			return "", fmt.Errorf("%s requires the cxz state directory", ShareVariable)
		}
		share, err := filepath.Abs(ShareDir(root))
		if err != nil {
			return "", err
		}
		src = strings.ReplaceAll(src, ShareVariable, share)
	}
	if strings.HasPrefix(src, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		src = filepath.Join(home, src[2:])
	}
	return filepath.Abs(src)
}
