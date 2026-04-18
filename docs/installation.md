---
title: Installation
description: Install Axel on macOS, Linux, or Windows
---

# Installation

## macOS / Linux

Run the install script:

```sh
curl -fsSL https://raw.githubusercontent.com/struckchure/axel/main/scripts/install.sh | bash
```

The script places the `axel` binary in `~/.local/bin` (Linux) or `/usr/local/bin` (macOS) and adds it to your `PATH`.

Verify the installation:

```sh
axel version
```

## Windows

Open PowerShell and run:

```powershell
irm https://raw.githubusercontent.com/struckchure/axel/main/scripts/install.ps1 | iex
```

The script downloads the binary to `%LOCALAPPDATA%\axel\` and adds it to your user PATH.

Restart your terminal, then verify:

```powershell
axel version
```

## Build from source

Requires Go 1.21+.

```sh
git clone https://github.com/struckchure/axel.git
cd axel
go build -o axel ./cmd
```

Move the binary somewhere on your `PATH`:

```sh
mv axel /usr/local/bin/axel   # macOS / Linux
```

## Configuration

Create an `axel.yaml` in your project directory:

```yaml
database-url: postgres://user:pass@localhost:5432/mydb
schema-path: ./schema.asl
migrations-dir: ./migrations
```

Or pass the project directory with `--dir` and Axel discovers config automatically:

```sh
axel -d . validate
axel -d . up
```

Discovery order inside `--dir`:

1. `axel.yaml` — loaded as the full config if found
2. `schema.asl` — used as the schema if no `axel.yaml`
3. `default.asl` — fallback schema filename

You can also set the database URL via an environment variable:

```sh
export DATABASE_URL=postgres://user:pass@localhost:5432/mydb
axel up
```
