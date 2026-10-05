# CI builds and edge publication

CI runs the full Go tests, Docker integration tests, race tests, generated-code checks,
web/browser/sandbox tests, and native Windows frontend checks. Documentation-only
changes retain the existing workflow filters.

## Build and cache ownership

`test`, `web`, `windows-frontend`, and `build` each own a Go cache. Keys include the
runner OS/architecture, job name, Go module files, and commit. A new commit restores
the latest compatible cache for that job, then saves the populated result. This
prevents a fast web job from permanently supplying an incomplete cache to Go tests
or cross-platform release builds. Race tests still use `-count=1`.

The `build` job runs in parallel with tests. It builds Linux and Windows binaries
for amd64 and arm64 once, including the source revision, and packages the download
archives and checksums. An artifact tar preserves executable permissions. Image
validation and publication consume that same artifact; they do not compile Go
again. The artifact is retained for three days; reruns after expiry need the build
job rerun too.

The `image` check assembles both manager image architectures without publishing.
Only after all checks succeed does main call `publish-edge.yml`.

## Publication ordering

Main validation runs can overlap. The reusable publication workflow has one
repository-wide concurrency group, spanning the freshness check, image publication,
release assets/manifest/tag update, cleanup, and the live Windows updater check.
Running publication is never cancelled. GitHub may replace a pending publication
with a newer pending one; intermediate builds need not all be released.

Before mutating any shared tag or release, `check-edge-publication.py` compares the
caller's CI run number with the published edge manifest. An older successful run
that finishes late is skipped. A retry of the same revision/sequence is allowed.
Unreadable or inconsistent existing publication metadata fails closed. Keep the
CI workflow's run-number sequence when changing this pipeline; moving edge builds
to a different caller workflow requires an explicit sequence migration.

The image is assembled again in the publication workflow using the already-built
binaries and Docker layer cache, then pushed. This keeps image writes under the same
lock as release mutation. Uploads still precede the manifest, and the Windows update
check completes before the next publisher starts.

## Timing comparisons

Compare PR checks separately from main publication. Record initial queue delay,
job duration, cache restore/save duration, and build/race-test steps. The first run
with new cache keys is cold; assess warm-cache behavior on subsequent commits.
No test coverage was removed to shorten the critical path.
