# Skill management

An [Agent Skill](https://agentskills.io/specification) is a directory holding a
`SKILL.md` with YAML frontmatter (`name`, `description`) and optional
`scripts/`, `references/` and `assets/`. Both supported agents read skills from
their own configuration directory, which cxz gives each session, so one library
serves either of them and nothing has to be declared per provider.

cxz stores the library once and decides per project which skills are delivered.
A skill nobody enabled reaches nobody.

## Where skills live

The library is under the share directory the installation already uses for
user-managed sources:

```
${CXZ_SHARE_DIR}/skills/<name>/SKILL.md
```

`cxz share` edits files there, so a skill is written in place and never
uploaded. `<name>` must match the `name` in the frontmatter — lowercase
letters, digits and single hyphens — and `.system` is refused because Codex
installs its own skills under that name.

Registration reads the frontmatter and records the description:

```sh
cxz skill add code-review          # register what is already in the library
cxz skill list                     # global defaults
cxz skill list --project web       # what that project sees, and why
```

## Scope

A skill has a global default and an optional per-project decision:

```sh
cxz skill enable code-review               # on wherever a project has not decided
cxz skill disable code-review --project web  # this project opts out
cxz skill enable deploy --project web        # this project opts in
cxz skill inherit deploy --project web       # follow the global default again
```

`list` reports each skill as `on`/`off` with `(inherited)` or `(project)`, so a
project's own decision is distinguishable from the default it follows.

**New registrations start disabled.** A skill's name and description are loaded
into every session's context whether or not the skill is used, so switching one
on everywhere is a decision rather than the default. Changing a global default
affects only the projects still inheriting it.

## Delivery

The manager resolves the list for a project and pushes only those skills,
alongside file mappings and MCP configuration, before a session is resumed or
created. An unactivated skill costs the project nothing, not even the bytes.
Delivery is bounded by the same budget as file mappings: 512 files and 2 MiB
per project.

Skills land in the session's agent configuration directory at
`skills/<name>/`, with the executable bit preserved so `scripts/` runs. They
apply at the **next agent start**, like the other pushed preferences; saving
never restarts an agent.

cxz owns that directory. A name it did not deliver is one it delivered before
and no longer does, so it is removed — turning a skill off takes it away rather
than leaving it behind. Dot-prefixed entries are left alone, which is where
Codex keeps its own `.system` skills.

A project runtime older than the manager does not know the push and is skipped
rather than treated as a failure; the skills arrive when that runtime updates.

## What this is not for

Skills that belong to a repository and can be committed should live in the
repository, where both agents already read them, version control covers them,
and people not using cxz still get them. This library is for the ones that
cannot be committed: personal workflow, organisation-internal procedure, or a
skill for a repository you do not own.
