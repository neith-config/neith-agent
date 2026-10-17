# neith-agent

`neith-agent` is a process launcher that fetches configuration from an external source and injects it into the target process environment. It bridges configuration management systems with arbitrary programs by transparently providing config values as environment variables and/or JSON files.

## Table of Contents

- [Quick Start](#quick-start)
- [Configuration Delivery](#configuration-delivery)
- [Command-Line Flags](#command-line-flags)
- [File Paths and XDG Compliance](#file-paths-and-xdg-compliance)
- [Environment Variable Injection](#environment-variable-injection)
- [Exit Codes](#exit-codes)
- [Platform-Specific Behavior](#platform-specific-behavior)
- [Examples](#examples)
- [JSON Output Format](#json-output-format)
- [Error Handling](#error-handling)

## Quick Start

The most basic usage launches a command with injected configuration:

```sh
neith-agent -- env
```

This will:
1. Fetch configuration from the default configuration source
2. Inject all config values as environment variables
3. Write configuration to `~/.config/neith-agent/env.json` (or `$XDG_CONFIG_HOME/neith-agent/env.json`)
4. Execute the `env` command with the injected environment

To suppress configuration injection:

```sh
neith-agent --no-env -- env                  # Skip environment injection
neith-agent --no-json-output -- env             # Skip JSON file writing
neith-agent --no-env --no-json-output -- env    # Skip child env injection and secondary JSON output
```

To use a custom path for the JSON file:

```sh
neith-agent --json-output /custom/path/config.json -- env
```

## Configuration Delivery

By default, neith-agent delivers fetched configuration in **two application-facing ways simultaneously**. The daemon's canonical shared snapshot remains available separately for dynamic delivery, including when `--no-json-output` is used:

### 1. Environment Variable Injection

All configuration entries are merged into the child process's environment as `KEY=value` pairs. The configuration completely overlays the parent's environment:
- All parent environment variables are inherited
- New config keys are added
- Existing keys (same name) are overwritten with config values

**Note**: If the parent environment contains duplicate keys (unusual but possible), only the first occurrence is replaced.

### 2. JSON File Output

Configuration is also written as a pretty-printed JSON file at:
- **Default location**: `$XDG_CONFIG_HOME/neith-agent/env.json`
- **XDG_CONFIG_HOME resolution** (in order):
  1. Value of `$XDG_CONFIG_HOME` environment variable (if set)
  2. `~/.config` (on Linux, macOS)
  3. `%APPDATA%` (on Windows)

The JSON file is created with permissions `0o600` (read/write for owner only) for security.

Both delivery mechanisms are **independent**:
- You can use only environment variables
- You can use only JSON file
- You can use both (default)
- You can use neither (with `--no-env --no-json-output`)

## Command-Line Flags

All flags must appear **before** the `--` separator. Arguments after `--` are passed to the target command.

### Format

```
neith-agent [FLAGS] -- COMMAND [ARGS...]
```

### Available Flags

| Flag | Description | Default | Effect |
|------|-------------|---------|--------|
| `--no-env` | Skip environment variable injection | false | Configuration is NOT merged into child environment |
| `--no-json-output` | Skip JSON file creation | false | Configuration JSON file is NOT written |
| `--json-output PATH` | Custom path for secondary JSON config file | `$XDG_CONFIG_HOME/neith-agent/env.json` | Mutually exclusive with `--no-json-output` |

### Flag Ordering

Flags can appear in any order before `--`:

```sh
# All equivalent:
neith-agent --no-env --json-output /tmp/cfg.json -- cmd
neith-agent --json-output /tmp/cfg.json --no-env -- cmd
```

### Special Behavior

- **`--json-output` and `--no-json-output`**: These flags are mutually exclusive. Supplying both is a command-line error; choose either the default path/custom secondary output or explicit suppression.

- **Invalid flags**: Unknown flags cause immediate error with exit code `2`
  ```sh
  neith-agent --unknown-flag -- cmd  # Error: exit code 2
  ```

- **Missing `--` separator**: The separator is **required**. Without it, exit code is `2`
  ```sh
  neith-agent env          # Error: missing -- separator
  neith-agent -- env       # OK
  ```

- **Missing command after `--`**: The command is required. Using only `--` causes error with exit code `2`
  ```sh
  neith-agent --          # Error: missing command
  neith-agent -- env      # OK
  ```

## File Paths and XDG Compliance

### Default JSON Output Location

neith-agent follows the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html):

```
$XDG_CONFIG_HOME/neith-agent/env.json
```

**Resolution order** (first match wins):

1. **If `$XDG_CONFIG_HOME` is set**: `$XDG_CONFIG_HOME/neith-agent/env.json`
2. **On Linux/macOS** (fallback): `~/.config/neith-agent/env.json`
3. **On Windows** (fallback): `%APPDATA%/neith-agent/env.json`

**Examples**:

| Scenario | Location |
|----------|----------|
| Linux, `$XDG_CONFIG_HOME=/opt/conf` | `/opt/conf/neith-agent/env.json` |
| Linux, `$XDG_CONFIG_HOME` unset | `~/.config/neith-agent/env.json` |
| macOS, `$XDG_CONFIG_HOME` unset | `~/.config/neith-agent/env.json` |
| Windows 10, `%APPDATA%=C:\Users\user\AppData\Roaming` | `C:\Users\user\AppData\Roaming/neith-agent/env.json` |

### Custom Path

Use `--json-output PATH` to override the default location completely. The path can be:
- Absolute: `/etc/app/config.json`
- Relative: `./config.json` (relative to current working directory)
- With variables: `$HOME/my-config.json` (note: **not expanded by neith-agent**, expanded by shell)

```sh
# Shell expands $HOME before passing to neith-agent
neith-agent --json-output $HOME/.config/myapp.json -- cmd

# Absolute path
neith-agent --json-output /etc/app/config.json -- cmd

# Relative path (relative to current directory)
neith-agent --json-output ./config/env.json -- cmd
```

### Directory Creation

If parent directories don't exist, neith-agent automatically creates them with permissions `0o755` (rwxr-xr-x).

If directory creation fails (permission denied, read-only filesystem, etc.), the program exits with code `1`.

### File Permissions

- **Directory**: `0o755` (rwxr-xr-x)
- **JSON file**: `0o600` (rw-------) — only the owner can read/write

This is intentional for security: configuration files often contain sensitive data.

### Error Handling for File I/O

If the JSON file cannot be written:
- The program exits with code `1`
- The target process is **NOT launched**
- Reason: You explicitly requested file writing; failure is not silent

If environment injection fails (rare), the program also exits with code `1`.

## Environment Variable Injection

### How Merging Works

The target process environment is constructed as:

1. **Start with**: All parent environment variables (inherited from neith-agent's parent)
2. **Merge in**: All fetched configuration key-value pairs
3. **Behavior**:
   - New keys from config are **added**
   - Existing keys are **overwritten** with config values
   - Keys from parent not in config are **preserved**

**Example**:

```bash
# Parent environment
export LANG=en_US.UTF-8
export PATH=/usr/bin:/bin
export APP_DEBUG=false

# Configuration fetched (e.g., {"APP_DEBUG": "true", "API_KEY": "secret123"})

# Child process environment
LANG=en_US.UTF-8
PATH=/usr/bin:/bin
APP_DEBUG=true        # ← overwritten by config
API_KEY=secret123     # ← new from config
```

### Empty or Missing Config

If configuration is empty (no entries), the child process receives the parent's environment unchanged.

### Special Characters in Values

Configuration values are used as-is. Special characters and whitespace are preserved:

```json
{
  "COMPLEX_VAR": "value with spaces and \"quotes\"",
  "PATH_VAR": "/usr/bin:/usr/local/bin"
}
```

These are safely injected into the environment — no shell escaping needed internally.

### Duplicate Keys in Parent Environment

If the parent environment contains duplicate keys (unusual but technically possible):

```
MYVAR=value1
MYVAR=value2  # duplicate
```

The config value replaces only the **first** occurrence. The second remains (this is a known limitation).

## Exit Codes

neith-agent returns specific exit codes for different failure modes. Success returns the target process's exit code.

| Exit Code | Meaning | Example |
|-----------|---------|---------|
| **0** | Success — target process exited with code 0 | `true` command exits 0 |
| **1** | Configuration error | Fetch failed, JSON write failed, env setup failed |
| **2** | Command-line parsing error | Invalid flags, missing `--`, missing command |
| **126** | Permission denied | Target command exists but is not executable |
| **127** | Command not found | Target command doesn't exist or isn't in `$PATH` |
| **N** (other) | Target process exit code | Target process exited with code N |

### Examples

```sh
neith-agent -- true
echo $?  # 0 (success)

neith-agent -- false
echo $?  # 1 (command returned 1)

neith-agent -- sh -c 'exit 42'
echo $?  # 42 (arbitrary process exit code)

neith-agent --invalid-flag -- true
echo $?  # 2 (flag parsing failed)

neith-agent -- /nonexistent/command
echo $?  # 127 (command not found)

neith-agent --no-json-output --json-output /root/config.json -- true
echo $?  # 1 (permission denied, can't write /root/config.json)
```

## Platform-Specific Behavior

### Unix/Linux Behavior

#### Signal Forwarding

When you press Ctrl+C or send another signal to neith-agent, it forwards the signal to the child process **and its entire process group**.

**What this means**:
- `Ctrl+C` kills the child and any grandchildren it spawned
- Child process group isolation is automatic
- Signal forwarding is transparent (no visible delay)

**Signals forwarded**: All POSIX signals except `SIGKILL` and `SIGSTOP` (which cannot be caught anyway)

**Implementation detail**: Child process is placed in its own process group using `Setpgid` syscall. Signals are sent to `-pid` (negative PID = process group) so all descendants receive them.

#### File Paths

```
$XDG_CONFIG_HOME/neith-agent/env.json
~/.config/neith-agent/env.json        (if XDG_CONFIG_HOME not set)
```

### Windows Behavior

#### Signal Forwarding

Windows doesn't support POSIX signals or process groups. Only `Ctrl+C` (os.Interrupt) is supported.

**What this means**:
- Pressing `Ctrl+C` sends `os.Interrupt` to the child process
- Only direct children receive the signal
- Grandchildren are not automatically terminated

**Supported signals**: 
- `os.Interrupt` (Ctrl+C) ✓
- Other POSIX signals: Not supported

**Workaround**: Use job objects or `taskkill` if you need to terminate process trees on Windows.

#### File Paths

```
%APPDATA%/neith-agent/env.json        (e.g., C:\Users\user\AppData\Roaming\neith-agent\env.json)
```

If `XDG_CONFIG_HOME` is set (even on Windows), it's still used as the base directory.

#### Path Separators

Paths use forward slashes `/` internally (Go standard), which works on all platforms. No manual conversion needed.

### Cross-Platform Considerations

- **Newlines**: Configuration values preserve platform-specific newlines (CRLF on Windows, LF on Unix)
- **Case sensitivity**: Filenames are case-sensitive on Unix, case-insensitive on Windows
- **Permissions**: Unix file permissions are applied on Unix; ignored on Windows

## Examples

### Example 1: Basic Configuration Injection

Inject config and run a command:

```sh
neith-agent -- env | grep APP_
# Output includes any APP_* variables from config
```

### Example 2: Skip JSON File (Only Environment)

```sh
neith-agent --no-json-output -- python my_app.py
# Config injected as environment variables
# JSON file not created
```

### Example 3: Custom JSON Output Location

```sh
neith-agent --json-output /tmp/app_config.json -- node server.js
# JSON written to /tmp/app_config.json
# Config also injected as environment variables
```

### Example 4: Only JSON Output (No Environment)

```sh
neith-agent --no-env --json-output /etc/app/config.json -- bash -c \
  'jq . /etc/app/config.json | grep api_key'
# Config NOT in environment
# JSON written to /etc/app/config.json
# Child process reads JSON file instead
```

### Example 5: Just Run (No Injection)

For testing or debugging:

```sh
neith-agent --no-env --no-json-output -- ps aux
# Runs ps with parent's environment unchanged
# No config injection at all
```

### Example 6: Pass Arguments to Command

Arguments after the command are passed through:

```sh
neith-agent -- curl https://api.example.com --header "X-API-Key: secret"
# neith-agent: executes curl
# curl: receives --header and X-API-Key argument
```

### Example 7: With Shell (Complex Commands)

For shell operators (pipes, redirects), wrap in shell:

```sh
neith-agent -- sh -c 'echo $DATABASE_URL | tee config.log'
# Works because sh parses the whole command line
```

### Example 8: Handle Relative Paths

```sh
cd /app
neith-agent --json-output ./config.json -- node index.js
# JSON written to /app/config.json (relative to cwd)
```

## JSON Output Format

The JSON file is always pretty-printed with 2-space indentation:

```json
{
  "DATABASE_URL": "postgresql://user:pass@localhost/db",
  "API_KEY": "secret123",
  "DEBUG": "true",
  "CACHE_TTL": "3600"
}
```

### Format Guarantees

- **Order**: Keys are in arbitrary order (Go maps are unordered)
- **Escaping**: All special characters are properly JSON-escaped
- **Newlines**: Trailing newline is included
- **Encoding**: UTF-8 (default for JSON)

### Parsing the JSON

Any JSON parser can read the file:

```bash
# With jq
jq .DATABASE_URL /path/to/config.json

# With Python
python -c "import json; cfg = json.load(open('/path/to/config.json')); print(cfg['API_KEY'])"

# With Node.js
node -e "console.log(require('/path/to/config.json').API_KEY)"
```

## Error Handling

### Fetch Failure

If configuration cannot be fetched from the source (network error, permission denied, etc.):
- Exit code: `1`
- Child process is **NOT launched**
- No JSON file is written

### JSON Write Failure

If the JSON file cannot be written (no permission, disk full, invalid path):
- Exit code: `1`
- Child process is **NOT launched**
- The program fails fast to prevent silent failures

### Directory Creation Failure

If parent directories for the JSON file cannot be created:
- Exit code: `1`
- Child process is **NOT launched**

**Note**: neith-agent only attempts to create parent directories, not deeper arbitrary paths.

### Command Not Found

If the target command doesn't exist or isn't in `$PATH`:
- Exit code: `127` (standard shell convention)
- Configuration was already injected (to environment and/or JSON)
- No error is printed by neith-agent (just the exit code)

### Permission Denied

If the target command exists but isn't executable:
- Exit code: `126` (standard shell convention)
- Configuration was already injected
- No error is printed by neith-agent

### Ctrl+C / Signal Handling

When you press Ctrl+C:
1. neith-agent receives the signal
2. Signal is forwarded to the child process (and process group on Unix)
3. Both neith-agent and child process terminate
4. Exit code reflects the signal (often 130 for SIGINT, varies by OS)

The mechanism is transparent — neith-agent itself doesn't "catch" Ctrl+C; it proxies signals.

---

**For questions or issues, check the project repository.**
