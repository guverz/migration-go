# migration-go

CLI for managing database migration files across a Git repository and its Git submodules. It creates versioned migration pairs, validates consistency between the project catalog and submodule sources, and synchronizes collected copies with `collect`.

## Requirements

- Go 1.25+
- Git repository with tags (used for project name and version in `add`)
- Optional: [GoReleaser](https://goreleaser.com/) for release builds

## Installation

### Linux

Install the latest release binary with the install script:

```bash
curl -fsSL https://raw.githubusercontent.com/guverz/migration-go/main/install.sh | bash
```

### Windows

Install via [Scoop](https://scoop.sh/):

```powershell
# Add the bucket (if not already added)
scoop bucket add guverz https://github.com/guverz/scoop-bucket

# Install migration-go
scoop install guverz/migration-go

# Update migration-go
scoop update guverz/migration-go
```

### From source

```bash
git clone https://github.com/guverz/migration-go.git
cd migration-go
go build -o migration-go .
```

Run from the repository root (or any directory that contains `config.yaml` or a configured migrations path):

```bash
./migration-go check
```

### Pre-built binaries (GoReleaser)

Releases are built with [GoReleaser](https://goreleaser.com/) on tag push (`v*`). Archives are published for:

| OS      | Architectures   |
|---------|-----------------|
| Linux   | `amd64`, `arm64` |
| Windows | `amd64`, `arm64` |
| macOS   | `amd64`, `arm64` |

Download the archive for your platform from [GitHub Releases](https://github.com/guverz/migration-go/releases), extract `migration-go` (or `migration-go.exe` on Windows), and place it on your `PATH`.

To build release artifacts locally:

```bash
goreleaser build --snapshot --clean
```

Binaries are written to `dist/`.

## Configuration

Configuration is loaded from (first match wins):

1. File passed with `--config /path/to/config.yaml`
2. `./config.yaml` in the current directory
3. `$HOME/config.yaml`

If no file is found, defaults are used and a message is printed to stderr.

### `config.yaml` keys

| Key | Default | Description |
|-----|---------|-------------|
| `help.include` | `true` | When `add` creates a new migration pair, append the contents of the mini-help template file into both `.up.sql` and `.down.sql`. |
| `directories.mini_help` | `migration.template.sql` | Path to the template file injected by `add` when `help.include` is `true`. |
| `directories.migrations` | `./migrations` | Project migration catalog directory (top-level `.up.sql` / `.down.sql` pairs and include files). |

Example:

```yaml
help:
  include: true

directories:
  mini_help: "migration.template.sql"
  migrations: "./migrations"
```

Environment variables override YAML keys (Viper `AutomaticEnv`), e.g. `DIRECTORIES_MIGRATIONS=./custom/migrations`.

## Global flags

| Flag | Description |
|------|-------------|
| `-h`, `--help` | Print help and exit. |
| `-V`, `--version` | Print CLI version and exit. |
| `--config` | Path to a YAML config file. |
| `--no-color` | Disable colored log output. |
| `-d`, `--debug` | Enable debug messages (`DEBUG:` prefix). |

Subcommand: `migration-go version` — prints `version`, `commit`, and `build date` from linker flags (set by GoReleaser in release builds).

## Commands

Run all commands from the project root (where `.gitmodules` and the migrations catalog live).

### `add`

Creates a new **up/down migration pair** in `directories.migrations`.

**Arguments:** none.

**Behavior:**

1. Reads **project**, **version**, and **release** from Git via [version-go](https://github.com/AlexBurnes/version-go) (same semantics as the legacy `describe` shell helper).
2. Builds base name `{project}-{version}-{release}`.
3. Finds the highest existing increment `N` for files matching `{base}-N.up.sql`.
4. Creates `{base}-{N+1}.up.sql` and `{base}-{N+1}.down.sql`.

**Example:**

```text
$ migration-go add
Add migration script myapp-1.2.3-1
Created migration files:
   ./migrations/myapp-1.2.3-1-3.up.sql
   ./migrations/myapp-1.2.3-1-3.down.sql
```

New files start with a comment header (`# filename.up.sql` / `# filename.down.sql`). If `help.include` is true, the mini-help template is appended.

---

### `check`

Validates the project migration catalog against Git submodule migration directories. **Does not modify files**; if fixes are needed, run `collect`.

**Arguments:** none.

**Scans:**

- Top-level migration pairs in `directories.migrations`
- Top-level migration pairs in each submodule path from `.gitmodules` → `{submodule}/migrations/`
- `@include` references and `#migration:` metadata links between project copies and submodule originals

**Example (success):**

```text
$ migration-go check
[OK]: No errors!
```

**Example (issues found — exit code non-zero):**

```text
$ migration-go check
WARNING: there are unregistered migration files (1), collect them and commit:
        module/migrations/myapp-0.1.0-1-1.up.sql
WARNING: there is number of unregistered include files (1), collect them and commit:
        include tables/users.sql included by ./migrations/myapp-1.2.3-1-2.up.sql
Error: use collect command
```

Other messages:

| Level | Meaning |
|-------|---------|
| `ERROR` | Incomplete pairs in a submodule (must be fixed manually). |
| `WARNING` | Drift that `collect` can reconcile (missing copies, obsolete includes/files, missing counterparts in the project catalog). |

---

### `collect`

Synchronizes the project migration catalog with submodule sources: registers new module migrations, copies updated include files, recreates missing project-side pairs, and removes obsolete project files/includes when safe.

**Arguments:** none.

**Example (work done):**

```text
$ migration-go collect
there are unregistered migration files pairs (1), collecting:
myapp-0.1.0-1-1.up|down.sql
creating new migration file
pair myapp-0.1.0-1-1.up|down.sql updating migration pair: myapp-1.2.3-1-4.up|down.sql
[OK]: collected 2 file(s)
```

**Example (nothing to do):**

```text
$ migration-go collect
[OK]: nothing to collect
```

**Typical workflow:**

```bash
migration-go check    # report drift
migration-go collect  # apply automatic fixes
git diff              # review and commit
```

## Migration file syntax

Migration files are plain text (usually `.sql`). This CLI **parses structure and metadata**; it does **not** execute SQL against a database.

### Special lines (parsed by migration-go)

| Prefix | Purpose |
|--------|---------|
| `#` | Comment. First line is often `# {filename}` (added by `add`). |
| `#migration:` | Metadata linking a **project** migration file to its **submodule** source (see below). |
| `@` | Include directive — pull in another file (see [@include](#include-system)). |

### SQL and scripts

Body lines (not starting with `#` or `@`) are opaque to this tool and are intended for your database migration runner.

**Statements** are typically separated by:

- A semicolon `;` at the end of a line for single SQL statements, or
- A slash `/` on its own line for PL/SQL blocks (Oracle-style).

**Scripts** (DDL from `install.sql`, table scripts, etc.) are usually referenced via `@include` rather than duplicated inline. Include files may contain one or more statements using the same delimiter rules.

Optional template `migration.template.sql` (see `directories.mini_help`) can document project-specific conventions for authors; it is only injected by `add`, not read by `check` / `collect`.

### Example migration file (submodule — original)

```sql
# myapp-0.1.0-1-1.up.sql

@tables/users.sql

CREATE INDEX idx_users_name ON users (name);
```

### Example migration file (project — collected copy)

```sql
# myapp-1.2.3-1-4.up.sql
#migration: module/migrations/myapp-0.1.0-1-1.up.sql;abc123...def456...

@tables/users.sql

CREATE INDEX idx_users_name ON users (name);
```

## Include system

An include line starts with `@` followed by a **relative path** (no spaces). The path is resolved from the directory of the file that contains the `@` line.

```text
@tables/users.sql
@includes/common_grants.sql
```

**Rules:**

- Recursive includes are supported.
- Include loops are detected and reported as errors.
- Missing include files are recorded during parsing (`check` may warn; submodule-side missing includes often require manual fixes).
- During `collect`, include files referenced from submodule migrations but missing in the project catalog are copied under `directories.migrations`, preserving the relative path below `migrations/` (e.g. `migrations/tables/users.sql`).

**Example layout:**

```text
migrations/
  myapp-1.2.3-1-1.up.sql      # contains: @tables/users.sql
  tables/
    users.sql                  # symlink or copy of ../ddl/tables/users.sql
```

## Migration file naming

Pattern:

```text
{name}-{version}-{release}-{increment}.{up|down}.{ext}
```

| Part | Description |
|------|-------------|
| `name` | Project name from Git (`version-go`) |
| `version` | Semantic version from Git tags |
| `release` | Release/build counter from `version-go` |
| `increment` | Auto-incremented serial per `{name}-{version}-{release}` prefix (`add`) |
| `up` / `down` | Apply or rollback script |
| `ext` | Extension, usually `sql` (other extensions are allowed if they match the pattern) |

**Examples:**

```text
ClearingManager-mts-1.6.4.0-1-1.up.sql
ClearingManager-mts-1.6.4.0-1-1.down.sql
myapp-1.2.3~pre.6-1-2.up.sql
```

Regex used internally: `(.+\-[0-9\.\-]+)\.(up|down)\.([^\.]+)$`

Every migration must exist as a **pair**: for each `*.up.{ext}` there must be a matching `*.down.{ext}` with the same prefix and extension.

## Metadata: `#migration:path;md5`

Used in **project** migration files that are collected copies of submodule migrations.

```text
#migration: <relative-path-to-source>; <md5-up><md5-down>
```

| Field | Description |
|-------|-------------|
| `relative-path-to-source` | Path to the original `.up.sql` or `.down.sql` in a submodule (e.g. `roam-tap/migrations/ClearingManager-roam-tap-1.3.12~pre.6-1-1.up.sql`). |
| `md5-up` + `md5-down` | Hex MD5 of the **entire** up file content and down file content, concatenated into one string (not MD5 of the pair as a single blob). |

**Purpose:**

- Tie the project catalog entry to a specific submodule file.
- Detect when submodule content changed (hash mismatch → re-collect).
- Detect when the submodule file was removed (`check` / `collect` → obsolete project migration).
- Match module pairs to project copies during `check` and `collect`.

`collect` writes this line when registering a new pair from submodule files. Original submodule migrations do **not** contain `#migration:` lines.

## Directory structure requirements

The tool expects a **DDL project** layout with a central migration catalog and optional Git submodules. Under `migration/`, subdirectories often mirror DDL areas (`tables/`, `packages/`, …) via symlinks or copies to `../ddl/...` or `../tables/...`.

### Database schema project (DDL)

```text
|- install.sql       # full schema bootstrap script
|- tables/           # table DDL scripts
|- functions/        # function scripts
|- packages/         # package scripts
|- types/            # type scripts
...
|- migration/        # migration catalog
   |- tables/        # -> ../tables
   |- packages/      # -> ../packages
   ...
```

### Module layout (Git submodules)

```text
|- modules/                # Git submodules
|  |- ddl/                 # module DB structure
|  |  |- tables/
|  |  |- packages/
|  |  ...
|  |- library/             # other submodules
|- migration/              # platform/project migrations
   |- tables/              # -> ../ddl/tables
   |- packages/            # -> ../ddl/packages
   ...
```

Each submodule that ships migrations should contain:

```text
{submodule}/migrations/
  {name}-{version}-{release}-{n}.up.sql
  {name}-{version}-{release}-{n}.down.sql
```

Submodule paths are read from `.gitmodules` (`path = ...` entries).

### Platform (meta-package) layout

```text
|- ddl/                    # shared platform DB structure
|  |- tables/
|  |- packages/
|  ...
|- migration/              # platform migrations
|  |- tables/              # -> ../ddl/tables
|  |- packages/            # -> ../ddl/packages
|  ...
|- module1/                # Git submodule
|- module2/
|- module3/
...
```

Point `directories.migrations` in `config.yaml` at the platform or project `migration/` directory.

## Development

```bash
go test ./...
golangci-lint run    # if configured locally
```

Release pipeline: `.github/workflows/release.yaml` runs GoReleaser on version tags.

## License

This project is licensed under the Apache License, Version 2.0 - see the LICENSE file for details.
