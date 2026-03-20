## Composition Tool Specification

The composition tool is a Launchr plugin that manages platform composition. It fetches dependencies defined in a
`plasma-compose.yaml` file from Git, HTTP, or local path sources, resolves them recursively, and merges their contents
into a unified build directory (`.compose/build`) using configurable merge strategies.

### CLI

```
launchr compose [options]
```

Options:

- `-w, --working-dir` — directory where downloaded packages are stored (default: `.compose/packages`)
- `-s, --skip-not-versioned` — skip files not tracked by git when processing the platform directory
- `--conflicts-verbosity` — log file conflicts: for each conflict prints `[winning-package] - path > Selected from [source]`
- `--clean` — remove the packages directory before running (build directory is always recreated)
- `--interactive` — allow interactive credential prompts (default: `true`)

Example:

```
launchr compose --skip-not-versioned --conflicts-verbosity
launchr compose --clean
```

### Conflict resolution

When the same file exists in multiple packages (or in the platform directory and a package), the **last writer wins**:
the package listed later in `plasma-compose.yaml` takes precedence. The platform directory is always processed last,
so platform files win over all packages by default.

Merge strategies let you override this behavior per package and per path.

### `plasma-compose.yaml` File Format

```yaml
dependencies:
  - name: my-package
    source:
      type: git          # git | http | path
      url: https://github.com/example/my-package.git
      ref: main          # branch, tag, or commit (git); file path (path)
      strategy:
        - name: remove-extra-local-files
          paths:
            - path/to/dir/
        - name: ignore-extra-package-files
          paths:
            - config/local.yaml
```

#### Source types

| Type   | Description                                      |
|--------|--------------------------------------------------|
| `git`  | Clone/fetch a Git repository (default)           |
| `http` | Download and extract a `.tar.gz` or `.zip` archive |
| `path` | Copy from a local directory path                 |

#### Version conflict detection

If the same package name is required by two different packages with a different type, URL, or ref, the build fails
with a `version conflict` error. Packages with identical type, URL, and ref are deduplicated and downloaded once.

### Merge Strategies

Strategies are declared per dependency in `plasma-compose.yaml`. Each strategy applies to a list of paths (file or
directory prefixes).

#### `remove-extra-local-files`

Removes files from the platform directory that match the given paths before merging. Use this to clean up platform
files that should be fully replaced by a package.

```yaml
strategy:
  - name: remove-extra-local-files
    paths:
      - generated/
```

#### `ignore-extra-package-files`

Skips files from the package that match the given paths. Files outside the listed paths are merged normally.
Use this to prevent a package from overwriting specific local files.

```yaml
strategy:
  - name: ignore-extra-package-files
    paths:
      - config/local.yaml
      - inventories/dev.yaml
```

#### `filter-package-files`

Whitelist: only files from the package that match the given paths are included; all other package files are dropped.
Within the matching paths, last-writer-wins still applies (a later package or the platform can still overwrite).

```yaml
strategy:
  - name: filter-package-files
    paths:
      - roles/
      - playbooks/site.yml
```

> **Deprecated:** `overwrite-local-file` — this strategy had no effect since the default conflict resolution became
> last-writer-wins. It is accepted but ignored, and will be removed in a future version.

### Fetching and Installing Dependencies

The tool resolves and installs dependencies by:

1. Recursively reading `plasma-compose.yaml` files starting from the root.
2. For each package: checking if the local copy is up-to-date; fetching it if not.
3. If the fetched package contains its own `plasma-compose.yaml`, its dependencies are resolved first (depth-first).
4. Packages are deduplicated: the same package (identical type + URL + ref) is downloaded only once.
5. After all packages are fetched, files are merged into `.compose/build` in topological order (dependencies before
   dependents, YAML declaration order preserved for siblings).

### Plasma-compose commands

Manage the `plasma-compose.yaml` file using subcommands:

- `launchr compose:add` — add a new dependency
- `launchr compose:update` — update an existing dependency
- `launchr compose:delete` — remove one or more dependencies

For `compose:add` and `compose:update`, pass flags directly or run interactively without flags:

```
launchr compose:add --url https://github.com/example/pkg.git --type git
launchr compose:add --package my-package --url https://github.com/example/pkg.git --ref v1.0.0
launchr compose:update --package my-package --ref v2.0.0
launchr compose:delete my-package
```