package sessionattention

import (
	"context"
	"github.com/lesomnus/cxz/api"
)

type Entry struct {
	Seq             uint64
	Question, Error bool
}
type Update struct {
	Sessions []string
	Unread   map[string]Entry
	Alerts   []string
	Err      error
}
type Backend interface {
	WatchAttention(context.Context, []*api.Session, func(Update)) error
	MarkConversationRead(context.Context, string, uint64) error
}
