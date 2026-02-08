# CLAUDE.md

Plasma Compose — Launchr CLI plugin for dependency composition. Fetches packages from Git/HTTP sources, merges them with local files into `.compose/build/` using configurable conflict resolution strategies.

## Commands
```bash
make deps      # install go dependencies
make test      # run tests (go test ./...)
make lint      # golangci-lint v2.5.0
make build     # build binary
make all       # deps + test + build
```
`DEBUG=1` enables debug symbols. Requires Go 1.24.0+, CGO disabled.

## File Layout
| File | Role |
|------|------|
| `plugin.go` | Launchr plugin, registers actions: `compose`, `compose:add`, `compose:update`, `compose:delete` |
| `compose/compose.go` | Orchestrator: working dirs, cleanup, signal handling. Key type: `Composer` |
| `compose/downloadManager.go` | Downloads packages (Git/HTTP), recursive dependency resolution. Key type: `DownloadManager` |
| `compose/git.go` | Git clone/fetch with keyring auth. Uses `go-git` with `EnableDotGitCommonDir: true` for worktree support |
| `compose/http.go` | HTTP download + extraction (`.tar.gz`, `.zip` only) |
| `compose/builder.go` | Filesystem merge with topsort ordering + merge strategies. Key type: `Builder` |
| `compose/yaml.go` | Config parsing. Key types: `YamlCompose`, `Package`, `Dependency`, `Source`, `Strategy` |
| `compose/forms.go` | Interactive TUI forms for add/update/delete actions |
| `action.*.yaml` | Action definitions with CLI options |

## Critical Behaviors
- **Strategy path matching**: Uses `strings.HasPrefix` (prefix matching), NOT glob patterns. Trailing separator auto-appended.
- **Conflict default**: Local files win. Strategies override this per-path.
- **Layer order**: Local files first, then packages in topological sort order (dependencies before dependents).
- **First strategy wins**: Per file, first matching strategy determines action.
- **`remove-extra-local-files`**: Only strategy targeting local walk; others target package walk.
- **Git worktrees**: Supported via `PlainOpenWithOptions` with `EnableDotGitCommonDir: true`.
- **Excluded from build**: `.compose/` dir, `plasma-compose.yaml`, `.git` from packages (local `.git` preserved).
- **Auth cascade**: no-auth → keyring (global base URL) → keyring (exact URL) → manual prompt.

## Merge Strategies
| Strategy | Target | Effect |
|----------|--------|--------|
| `overwrite-local-file` | Package files | Package replaces local at matching paths |
| `remove-extra-local-files` | Local files | Skip local files matching paths (not added to build) |
| `ignore-extra-package-files` | Package files | Skip package files at matching paths |
| `filter-package-files` | Package files | Only include package files matching paths |

## Config Format
```yaml
name: project-name
dependencies:
  - name: package-name
    source:
      type: git          # git (default) or http
      url: https://github.com/user/repo.git
      ref: branch-or-tag # omit for default branch (target="latest")
      strategy:
        - name: overwrite-local-file
          path: ["config/", "templates/"]
```

## Working Directories
- `.compose/packages/<name>/<ref>/` — downloaded package contents
- `.compose/build/` — final composed output (cleaned each run)
- `--clean` flag removes entire `.compose/` before run
- `--skip-not-versioned` filters to git-tracked files only (works with worktrees)
