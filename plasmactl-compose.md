# Plasma Compose Reference

Plasma Compose is a Launchr CLI plugin that composes a filesystem from multiple package sources. It reads `plasma-compose.yaml`, fetches dependencies (Git/HTTP), resolves them via topological sort, merges files with configurable strategies, and outputs to `.compose/build/`.

## Configuration
```yaml
name: project-name
dependencies:
  - name: package-name
    source:
      type: git          # "git" (default) or "http"
      url: https://github.com/user/repo.git
      ref: main          # branch or tag; omit for default branch
      strategy:
        - name: overwrite-local-file
          path: ["config/", "templates/"]
```
**Git sources**: Clone by `ref` (tries tag first, then branch). Omitting `ref` clones the default branch (target=`latest`).
**HTTP sources**: Download and extract `.tar.gz` or `.zip` archives. `ref` is ignored.

## CLI Actions

**`compose`** — Full composition (fetch + build).
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `--working-dir, -w` | string | `.compose/packages` | Package download directory |
| `--skip-not-versioned, -s` | bool | false | Only include git-tracked files from source |
| `--conflicts-verbosity` | bool | false | Log conflict resolutions |
| `--clean` | bool | false | Remove `.compose/` before start |
| `--interactive` | bool | true | Allow interactive credential prompts |

**`compose:add`** — Add package to `plasma-compose.yaml`.
**`compose:update`** — Update package (or all interactively if `--package` omitted).
**`compose:delete`** — Remove packages (`--packages` comma-separated).
Add/update options: `--package`, `--type` (git/http), `--url`, `--ref`, `--strategy` (array), `--strategy-path` (array, pipe-separated paths). Add also has `--allow-create`.

## Composition Pipeline
1. **Parse** `plasma-compose.yaml` → `YamlCompose` (name + dependencies)
2. **Download** each dependency to `.compose/packages/<name>/<ref>/`
   - Recursively process nested `plasma-compose.yaml` in packages
   - Git: check local vs remote ref, skip if up to date
   - HTTP: skip if directory exists
3. **Clean** `.compose/build/`
4. **Walk local files** → entries tree (excludes `.compose/`, `plasma-compose.yaml`)
   - With `--skip-not-versioned`: only git-tracked files (works with worktrees)
   - `remove-extra-local-files` strategy applied here (matching paths skipped)
5. **Apply packages** in topological order (dependencies before dependents)
   - `.git` entries from packages are skipped
   - Per file: first matching strategy wins; default = local wins
6. **Copy** entries tree to `.compose/build/` preserving permissions and symlinks

## Merge Strategies
Strategy paths use **prefix matching** (`strings.HasPrefix`), not globs. Trailing separator is auto-appended.
| Strategy | Applied to | Behavior |
|----------|-----------|----------|
| `overwrite-local-file` | Package walk | Package file replaces local at matching paths |
| `remove-extra-local-files` | Local walk | Local files at matching paths excluded from build |
| `ignore-extra-package-files` | Package walk | Package files at matching paths silently skipped |
| `filter-package-files` | Package walk | Only package files at matching paths are included |

Per package, multiple strategies can be defined. First match wins, then processing stops for that file. If no strategy matches, default behavior applies (local files win on conflict).

## Authentication
Three modes tried in order for both Git and HTTP:
1. **None** — public access
2. **Keyring** — credentials from system keyring (first by base URL, then exact URL)
3. **Manual** — interactive prompt (requires `--interactive`)

Fails cascade to the next mode automatically.

## Directory Layout
```
project-root/
├── plasma-compose.yaml
└── .compose/
    ├── packages/<name>/<ref>/   # downloaded packages
    └── build/                   # composed output
```
`--clean` removes entire `.compose/`. Build dir is always cleaned before each run.

## Example
```yaml
name: enterprise-app
dependencies:
  - name: core-platform
    source:
      type: git
      url: https://github.com/corp/platform.git
      ref: v3.0.0
  - name: auth-module
    source:
      type: git
      url: https://github.com/corp/auth.git
      ref: v1.5.2
      strategy:
        - name: overwrite-local-file
          path: ["middleware/"]
  - name: monitoring
    source:
      type: http
      url: https://releases.corp.com/monitoring-v2.1.0.tar.gz
      strategy:
        - name: filter-package-files
          path: ["agents/", "config/monitoring/"]
```
**Result** (applied in order):
1. Local project files (base layer)
2. `core-platform` — all files merged, local wins on conflicts
3. `auth-module` — files under `middleware/` overwrite local versions
4. `monitoring` — only `agents/` and `config/monitoring/` included
