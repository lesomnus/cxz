package memoryview

import "github.com/lesomnus/cxz/internal/core"

const FileLimit = 256 * 1024
const EntryLimit = 1000

type Query struct {
	Session core.Session
	Path    string
}
type Entry struct {
	Name      string
	Directory bool
	Size      int64
}
type Page struct {
	Path, Location, Content, Note string
	Directory, Truncated          bool
	Entries                       []Entry
}

type CopyQuery struct {
	Source, Target   core.Session
	Path, TargetPath string
}
