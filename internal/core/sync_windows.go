package core

// Windows has no equivalent of POSIX directory fsync on an os.File directory
// handle. Callers sync the file contents before renaming; no directory flush is
// attempted by the frontend.
func SyncDir(string) error { return nil }
