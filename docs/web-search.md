# Web conversation search

Search lives at `/c/_/search`. It searches retained conversation messages for the
current cxz installation. It does not search workspace files, Memory documents or
every configured connection in the browser.

The page shares its search-bar styling with the in-conversation search overlay.
The resource sidebar and page title are omitted. Project selection and two Nox
DatePicker controls sit directly below the input. Search modes include
case-sensitive matching, regular expressions and fuzzy matching; regular
expressions and fuzzy matching are mutually exclusive. Filters support one project
or all projects and a UTC date range with an exclusive end date. The web page
searches user/assistant messages and exposes no Include tools option.
The date controls display `YYYY-MM-DD` and initially cover the seven UTC calendar
days through today: the end boundary is tomorrow at 00:00 UTC. The project
selector has a fixed width, and there is no Reset filters button; project scope
can be changed to All projects and either date can be cleared independently.

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

In manager installations, project state lives in subdirectories of the manager
volume mounted into project containers. The supervisor writes a per-session
`conversation.db`; the manager reads that same file, including while the project
is stopped. It does not mirror complete event pages into its own database.
`conversations.db` is the separate, rebuildable search projection.

The searchable text is extracted as events are committed. A background seed can
rebuild historical data. Before answering, Search spends up to the default
three-second catch-up budget ingesting events already present in the installation's
local event store. It does not fetch project journals during the query. Gaps or
unavailable data are reported in progress/pending fields; absence of a hit is not
proof that an uncollected or pruned message never existed. Results default to user
and assistant text. The RPC can also search retained tool text via include_tools,
but the web page leaves it disabled.
The cursor fixes the time window across pages while newer messages arrive.

This differs from the in-conversation overlay, which searches currently loaded
messages locally. The terminal multi-connection client can additionally merge one
Search stream per configured installation (`internal/multiclient/search.go`);
that is a fan-out across connections, not across projects.
