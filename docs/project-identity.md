# Project identity

`Project.id` is the canonical project UUID. Registration allocates it before any
container is created. Stop, Start, Remove, container recreation and upgrades keep
the same UUID. Erase removes the project and its owned state; registering that
workspace again creates a new project with a new UUID.

`alias` is the editable user-facing handle. `container_id` identifies the current
Docker container. Session runtime IDs are independent of this project migration.

## Existing installations

Before this change, the Manager keyed projects by a workspace-path hash, and the
resource layer derived a UUID from that hash. Upgrades retain that exact resource
UUID; they do not allocate replacement project, session, task or binding IDs.

The Manager records the former handle as `StorageID` and replaces the registry
key with the UUID. Existing state directories, Docker volume names, container
ownership labels, Compose names, secret directories and mounts keep their
physical names. They are storage locations, not public project identities.
MCP and skill overrides and the conversation search index move to the UUID scope. A relative export alias keeps
new uploads visible through existing containers' asset mounts.

The manifest records the mapping before the registry transaction commits. A
restart after either write repeats the migration safely. No running container
is stopped and no conversation or volume contents are copied for this change.
Old project-scoped authentication bindings remain valid only for the same mapped
project, account and backend. Session manifests normalize the project UUID while
retaining the old binding scope; shared memory keeps its existing physical
directory.

## References and clients

New clients use UUID bytes in `ProjectRef.id`; CLI arguments and web URLs use the
written UUID. The deprecated `ProjectRef.runtime_id` remains an input bridge for
saved links and older references. Unary requests, nested filters and streaming
handshakes translate it before resource access. Browser project and Task links
resolve an old handle to the current project UUID.

`Project.runtime_id` is removed from the resource, generated clients and storage
schema. Its protobuf name and number are reserved, as are the removed Add and
Select fields. Applications reading the removed response field must update;
accepting a legacy reference does not restore that response field.
