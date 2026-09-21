package selfupdate

import (
	"fmt"
	"os"
)

func (r *Replacement) replace() error {
	// Windows permits renaming the running executable, but not deleting it.
	// Keep that image at the backup path until all its processes have exited.
	if err := os.Remove(r.Previous); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove previous backup (close any process still using it): %w", err)
	}
	if err := os.Rename(r.Target, r.Previous); err != nil {
		return err
	}
	if err := os.Rename(r.Candidate, r.Target); err != nil {
		if restoreErr := os.Rename(r.Previous, r.Target); restoreErr != nil {
			return fmt.Errorf("replace failed: %v; restore failed: %v; old executable remains at %s", err, restoreErr, r.Previous)
		}
		return err
	}
	return nil
}
