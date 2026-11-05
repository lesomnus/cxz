# Web conversation search

Search lives at `/c/_/search`. It searches retained conversation messages for the
current cxz installation. It does not search workspace files, Memory documents or
every configured connection in the browser.

The page shares its search-bar styling with the in-conversation search overlay.
The resource sidebar and page title are omitted. Project selection and the
expandable Filters button sit below the input. Search modes include case-sensitive
matching, regular expressions and fuzzy matching; regular expressions and fuzzy
matching are mutually exclusive. Filters support one project or all projects,
optional tool output and a UTC date range with an exclusive end date.

Submitting Enter or the search button calls `SessionService.Search` once. Changing
the query/options aborts the active request and clears stale results; it does not
submit a new request. The action becomes Stop while streaming. Result links open a
session at the matching message sequence. Pagination reuses the submitted query,
filters and server cursor.

## Server path

`server/lifecycle/session.go` translates the resource RPC into the installation's
runtime Search request. `internal/server/search.go` answers it using one query
against the SQLite `conversations.db` maintained by `internal/convindex`. It does
not send search requests to individual project supervisors or start containers.
Project and session restrictions are filters within this store.

The searchable text is extracted as events are committed. A background seed can
rebuild historical data. Before answering, Search spends up to the default
three-second catch-up budget ingesting events already present in the installation's
local event store. It does not fetch project journals during the query. Gaps or
unavailable data are reported in progress/pending fields; absence of a hit is not
proof that an uncollected or pruned message never existed. Results default to user
and assistant text; Include tools also searches the retained supported tool text.
The cursor fixes the time window across pages while newer messages arrive.

This differs from the in-conversation overlay, which searches currently loaded
messages locally. The terminal multi-connection client can additionally merge one
Search stream per configured installation (`internal/multiclient/search.go`);
that is a fan-out across connections, not across projects.
