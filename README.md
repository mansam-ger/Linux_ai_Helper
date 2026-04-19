# 🐨 Eugen – Local AI System Assistant for Linux

**Eugen** is a privacy-first, offline-capable AI assistant for the Linux command line. Built for modern Linux distributions (including Debian, Ubuntu, RHEL, Arch, SLES, and openSUSE), it runs entirely locally – no cloud, no external API calls, no data leakage.

Eugen translates natural language into precise Bash commands, analyzes system logs, creates execution plans, and performs deep system diagnostics – all powered by a local LLM via [Ollama](https://ollama.com).

I had very good results with the following setup:

nomic-embed-text:latest
nemotron-cascade-2:latest

---

## Table of Contents

- [Features](#-features)
- [Prerequisites](#-prerequisites)
- [Installation](#-installation)
- [Configuration](#-configuration)
- [Quick Start](#-quick-start)
- [Usage](#-usage)
  - [Interactive REPL](#interactive-repl)
  - [REPL Commands](#repl-commands)
  - [CLI Flags](#cli-flags)
- [Features in Detail](#-features-in-detail)
  - [Command Generation & Execution](#command-generation--execution)
  - [Safety System (Risk Scoring)](#safety-system-risk-scoring)
  - [Command Validation (man/help)](#command-validation-manhelp)
  - [Task Planner](#task-planner)
  - [Log Analysis](#log-analysis)
  - [Health Check](#health-check)
  - [Deep Diagnosis (supportconfig)](#deep-diagnosis-supportconfig)
  - [System Database (Offline Knowledge)](#system-database-offline-knowledge)
  - [RAG Vector Database](#rag-vector-database)
  - [Chat Memory & Export](#chat-memory--export)
- [Architecture](#-architecture)
- [Privacy & Security](#-privacy--security)
- [License](#-license)

---

## ✨ Features

| Feature | Description |
|---|---|
| **Natural Language → Bash** | Ask questions in plain language and receive exact Bash commands |
| **Interactive Execution** | Commands are only executed after explicit user confirmation |
| **3-Tier Risk Scoring** | Automatic detection of dangerous commands (`rm -rf`, `dd`, `mkfs`, …) |
| **Command Validation** | Suggested commands are verified against real `man`/`--help` output |
| **Task Planner** | Break complex tasks into executable command sequences |
| **Log Analysis** | Automatic reading and assessment of `journalctl` & `dmesg` errors |
| **Health Check** | Instant overview of load, RAM, disk, services, firewall |
| **Deep Diagnosis** | Full `supportconfig` analysis with AI-powered assessment (SUSE only) |
| **Offline System Database** | Index hardware, network & services once and use them permanently |
| **RAG Vector Database** | Embed your own documents (`.txt`, `.md`) as local knowledge |
| **Chat Memory** | Ring buffer of the last 10 exchanges for context-aware responses |
| **Chat Export** | Save conversations as Markdown files |
| **100% Offline** | No cloud connection, no data leakage – fully air-gapped (unless OpenAI backend is selected) |
| **Zero Dependencies** | Pure Go standard library, statically compiled single binary |
| **Snapshots (BTRFS)** | Automatic Btrfs snapshots via `snapper` before high risk commands |
| **Plugin System** | Connect your own bash scripts and tools to the AI |

---

## 📋 Prerequisites

- **Operating System:** Any modern Linux Distribution (Debian, Ubuntu, RHEL, Fedora, Arch, SUSE, etc.)
- **Go:** Version 1.26+ (only needed for compilation)
- **Ollama:** Local Ollama instance (default: `http://localhost:11434`)
- **LLM Model:** A model loaded in Ollama (default: `nemotron-cascade-2:latest`)
- **Embedding Model:** For RAG functionality (default: `nomic-embed-text`)

### Setting up Ollama

```bash
# Install Ollama (if not already present)
curl -fsSL https://ollama.com/install.sh | sh

# Download models
ollama pull nemotron-cascade-2:latest
ollama pull nomic-embed-text
```

---

## 🔧 Installation

### Building from Source

```bash
# Clone the repository
git clone https://github.com/mansam-ger/Linux_ai_Helper.git
cd Linux_ai_Helper

# Build a static binary (zero CGO)
make build
```

The compiled binary will be located at `bin/eugen`.

### Make the binary available system-wide (optional)

```bash
sudo cp bin/eugen /usr/local/bin/
```

You can run eugen from anywhere by typing ./eugen the data directory will be created automatically in the current directory.

### Ansible Deployment (Enterprise)

For system administrators deploying Eugen across multiple servers, an Ansible playbook is included in the repository. It automatically handles copying the binary and creating the required configuration directories with the correct permissions.

```bash
# From within the repository root
ansible-playbook ansible/install_eugen.yml
```

### Makefile Targets

| Target | Description |
|---|---|
| `make build` | Compile a static binary to `bin/eugen` |
| `make clean` | Remove the `bin/` directory |
| `make test` | Run all unit tests |

---

## ⚙ Configuration

On first launch, Eugen automatically creates the `/etc/eugen` and `~/eugen_data/` directories and generates a default configuration file `eugen.conf` inside `/etc/eugen`.

**Auto-Updates:** Eugen tracks its configuration layout via a internal `version` key. If you update Eugen to a newer binary and the configuration template changes (e.g. new backend features are added), Eugen will automatically upgrade your `eugen.conf` while preserving your custom values!
*Note: Since `/etc/eugen` is owned by root, you might need to run `sudo eugen` once after a binary update so it has the permissions to rewrite the file layout.*

### Directory Structure

```
/etc/eugen/
└── eugen.conf              # Main configuration file

~/eugen_data/
├── eugen_db.json           # System database (created after -p indexing)
├── my_knowledge.txt        # ← Place your own RAG documents here
├── runbook.md              # ← Place your own RAG documents here
├── plugins/                # ← Place your own Bash/Python executable admin tools here
└── chat_2026-04-11_*.md    # Exported chat sessions
```

### Configuration File (`eugen.conf`)

The file uses a simple `key = value` format. Multi-line values (e.g. prompts) are enclosed with `"""`:

```ini
# ---- General ----
assistant_name = Eugen

# ---- Inference Backend ----
backend = ollama

# ---- Ollama Settings ----
ollama_url = http://localhost:11434
ollama_model = nemotron-cascade-2:latest
ollama_embed_model = nomic-embed-text

# ---- Feature Flags ----
validation_enabled = true
rag_enabled = true

# ---- Prompt Templates (multi-line with """) ----
prompt_system = """
You are {name}, a highly intelligent system assistant...
Current system:
{context}
"""
```

### Configuration Options

| Key | Default | Description |
|---|---|---|
| `assistant_name` | `Eugen` | Name of the assistant (appears in prompts as `{name}`) |
| `backend` | `ollama` | Inference backend (`ollama`, `openai`) |
| `ollama_url` | `http://localhost:11434` | URL of the Ollama server |
| `ollama_model` | `nemotron-cascade-2:latest` | LLM model for text generation |
| `ollama_embed_model` | `nomic-embed-text` | Embedding model for RAG |
| `openai_url` | `https://api.openai.com/v1` | URL for OpenAI-compatible APIs (LM Studio, vLLM, etc.) |
| `openai_key` | `""` | API Key for the OpenAI backend |
| `openai_model` | `gpt-4o` | LLM model for the OpenAI backend |
| `validation_enabled` | `true` | Validate commands against man/help output |
| `rag_enabled` | `true` | Enable RAG vector database search |

### Customizing Prompt Templates

All prompts in `/etc/eugen/eugen.conf` support placeholders:

| Placeholder | Description |
|---|---|
| `{name}` | Replaced with the value of `assistant_name` |
| `{context}` | System context (OS, kernel, hostname, DB knowledge) |
| `{commands}` | Suggested commands (only in `prompt_validation`) |
| `{help}` | Real help output (only in `prompt_validation`) |
| `{journalctl}` | Journal errors (only in `prompt_log_analysis`) |
| `{dmesg}` | Kernel errors (only in `prompt_log_analysis`) |

---

## 🚀 Quick Start

```bash
# 1. Index system knowledge once (recommended)
eugen -p

# 2. Launch Eugen
eugen

# 3. Start asking questions:
➜ eugen> How much memory does this machine have?
➜ eugen> Install nginx for me
➜ eugen> Show me the last failed services
```

---

## 🖥 Usage

### Interactive REPL

After launching, Eugen enters an interactive loop (REPL). Type questions or instructions in natural language and receive answers with matching Bash commands.

```
Eugen - Lokaler SLES/openSUSE Assistent
Backend: Ollama | URL: http://localhost:11434 | Modell: nemotron-cascade-2:latest
✔️ System-Datenbank gefunden! Lade lokales Systemwissen...
ℹ RAG: Vektor-Datenbank mit 12 Wissens-Chunks erfolgreich geladen.

➜ eugen> _
```

When a response contains executable commands, you will be prompted before execution:

- **Low Risk** → `Execute? [J/n]`
- **Medium Risk** → `[J]a / [N]ein / [A]npassen / [E]rklären` (Yes / No / Modify / Explain)
- **High Risk** → Type `EXECUTE` to confirm the command

### REPL Commands

| Command | Description |
|---|---|
| `help` or `?` | Display the help page |
| `health` or `check` or `status` | Quick system health check (load, RAM, disk, services) |
| `analyze` or `logs` | Read `journalctl` & `dmesg` errors and let the AI analyze them |
| `diagnose` or `supportconfig` | Run a full SLES deep diagnosis |
| `plan <task>` | Create a step-by-step execution plan for a complex task |
| `db show` or `db list` | Display the contents of the local system database |
| `db add <text>` | Permanently add custom knowledge (e.g. infrastructure notes) to the DB |
| `save` or `export` | Save the current chat session as a Markdown file |
| `validation on/off` | Toggle man/help validation at runtime |
| `rag on/off` | Toggle RAG vector database search at runtime |
| `exit` or `quit` | Exit Eugen |

### CLI Flags

```bash
eugen [flags]
```

| Flag | Description |
|---|---|
| `-v` | **Verbose mode**: Show prompts, API payloads, and RAG scores |
| `-f <file>` | Load a log file into the system context (max ~50 KB) |
| `-p` | **Populate**: Index hardware, network & services into the local DB, then exit |
| `-r` | **Reset**: Clear the system database and exit |

#### Examples

```bash
# Verbose mode for debugging
eugen -v

# Feed a log file directly
eugen -f /var/log/messages

# Build the system database
eugen -p

# Reset the system database
eugen -r
```

---

## 🔍 Features in Detail

### Command Generation & Execution

Eugen automatically extracts executable commands from AI responses – both from fenced code blocks (` ``` `) and inline backticks. The detection pipeline intelligently filters:

- Pure comments and config file content are skipped
- Only commands whose binary actually exists in `$PATH` are suggested
- Multi-command responses present a numbered selection menu

```
ℹ Eugen suggested multiple commands:
 [1] systemctl status nginx
 [2] sudo zypper install -y nginx [Caution]
 [3] sudo systemctl enable --now nginx [Caution]

Which command to execute? (Number) or [A]ll / [X]Cancel:
```

### Safety System (Risk Scoring)

Every suggested command is automatically evaluated with a 3-tier risk scoring system:

| Level | Examples | Confirmation |
|---|---|---|
| 🟢 **Low** | `cat`, `ls`, `grep`, `systemctl status` | `[J/n]` (simple yes/no) |
| 🟡 **Medium** | `zypper install`, `systemctl restart`, `chmod` | `[J]a / [N]ein / [A]npassen / [E]rklären` |
| 🔴 **High** | `rm -rf`, `dd`, `mkfs`, `fdisk`, writes to `/etc/` | Must type `EXECUTE` to confirm |

For **medium risk** commands, you can use `[A]npassen` (modify) to manually edit the command before execution, or press `[E]rklären` to let the AI dynamically explain what all the used flags and parameters mean before you agree.

**BTRFS Auto-Snapshots:** If Eugen detects a Btrfs filesystem and `snapper`, it will interatively ask to create a pre-execution snapshot for all Medium and High risk commands! If something goes horribly wrong, you can rollback.

### Command Validation (man/help)

When `validation_enabled = true` (default), all suggested commands are automatically validated against the real `--help` output of the respective programs before execution. If a flag doesn't exist, the AI corrects the command based on the actual help output.

This feature can be toggled at runtime with `validation on` / `validation off`.

### Task Planner

For complex tasks requiring multiple steps:

```
➜ eugen> plan Set up an Nginx reverse proxy with SSL
```

Eugen creates a structured execution plan with numbered commands. You then have three options:

- **[A]usführen** (Execute) – Run all commands sequentially (with safety checks)
- **[E]xportieren** (Export) – Save the plan as a shell script (`eugen_plan.sh`)
- **[X]Abbrechen** (Cancel) – Discard the plan

The planner automatically leverages the local system context, zypper package search, and the RAG database.

### Log Analysis

```
➜ eugen> analyze
```

Reads the most recent critical system errors from:
- `journalctl -p 3` (last 40 errors)
- `dmesg` (last 20 kernel errors)

The errors are passed to the AI for analysis, which provides a structured summary with actionable fix suggestions.

### Health Check

```
➜ eugen> health
```

Runs an instant system health check and lets the AI comment on the results:

- Uptime & load average
- Memory & swap usage
- Disk space utilization
- Top processes by CPU
- Failed systemd services
- Btrfs root usage
- Firewall state
- SELinux state
- Critical kernel events (dmesg)

### Deep Diagnosis (supportconfig)

```
➜ eugen> diagnose
```

Runs a full SLES system diagnosis using `supportconfig` *(Note: This feature is only supported natively on SLES/openSUSE)*:

1. Checks if `supportutils` is installed (auto-installs if needed)
2. Offers two diagnosis levels:
   - **[1] Minimal** – ~1 minute, heavily aggregated
   - **[2] Complete** – Several minutes, full system dump
3. Extracts and filters the most relevant information from the tarball
4. Passes the extract to the AI for deep analysis

### System Database (Offline Knowledge)

The system database extends Eugen's knowledge with detailed hardware, network, and service information about your system.

```bash
# Index once
eugen -p
```

The database is stored as `~/eugen_data/eugen_db.json` and contains:
- **Hardware info**: CPU, RAM, disk layout
- **Network info**: Interfaces, IPs, routing
- **Services**: All active systemd units
- **Custom notes**: Manually added infrastructure knowledge

#### Adding Custom Knowledge

```
➜ eugen> db add Our proxy server runs on 10.0.1.50:3128
➜ eugen> db add The database on db01 uses PostgreSQL 15 with pgbouncer
```

These notes are permanently stored and automatically injected into the system context for every conversation.

### Plugin System (Admin Tools)

You can provide Eugen with custom executable scripts (Bash, Python, etc.) through the Plugin System. Eugen will recognize these tools and dynamically suggest or call them when solving your problems.

1. Create a script in `~/eugen_data/plugins/`. Ensure it has the executable bit set (`chmod +x`).
2. Add a `Description` comment at the top of the file so Eugen understands what it does.

```bash
# ~/eugen_data/plugins/reset-vpn.sh
#!/bin/bash
# Description: Resets the company Wireguard VPN interface and flushes the routing table.

systemctl restart wg-quick@wg0
```

When you start Eugen, it will parse all scripts in that folder and inject their metadata into its intelligence. You can then say: `➜ eugen> The VPN is stuck again` and Eugen will propose to execute `/root/eugen_data/plugins/reset-vpn.sh`!

### RAG Vector Database

Eugen can use local knowledge documents via Retrieval-Augmented Generation (RAG). Simply place `.txt` or `.md` files in the `~/eugen_data/` directory:

```bash
# Example: Add your own documentation
cp docs/network-runbook.md ~/eugen_data/
cp docs/backup-policy.txt ~/eugen_data/
```

On startup, all documents are automatically:
1. Split into paragraphs (chunking at ~1000 characters)
2. Vectorized through the embedding model
3. Searched via cosine similarity on every query

A document is only injected into context if at least one chunk achieves a relevance score ≥ 0.7, preventing irrelevant matches from polluting the context.

**Excluded** from ingestion: `eugen.conf` and `eugen_db.json`.

### Chat Memory & Export

Eugen remembers the last **10 exchanges** (20 messages) in a ring buffer. Older messages are automatically discarded to prevent LLM context overflow.

```
➜ eugen> save
✔️ Chat saved as Markdown at: ~/eugen_data/chat_2026-04-11_15-30-00.md
```

The exported file contains the full chat history in readable Markdown format.

---

## 🏗 Architecture

Eugen follows a strict **zero-bloat philosophy**: 100% Go standard library, zero external dependencies, a single statically compiled binary.

```
eugen/
├── cmd/eugen/main.go          # Entry point, CLI parsing, REPL loop
├── internal/
│   ├── config/                # Configuration management (eugen.conf parser)
│   ├── context/               # System context, zypper/rpm search, RAG ingest, vector DB
│   ├── inference/             # Backend interface (Backend, Message)
│   ├── ollama/                # Ollama HTTP client (net/http, encoding/json)
│   ├── executor/              # Command execution & heuristic risk scoring
│   ├── cmdvalidator/          # Command validation against man/help output
│   ├── planner/               # Task planner for complex multi-step tasks
│   ├── loganalyzer/           # journalctl & dmesg error log reader
│   ├── diagnostic/            # Health check & supportconfig diagnosis
│   └── sysdb/                 # Persistent JSON system database
├── /etc/eugen/                # Configuration directory (eugen.conf)
├── ~/eugen_data/              # Data directory (DB, RAG documents, exports)
├── Makefile                   # Build system
└── go.mod                     # Go module (zero external dependencies)
```

### Backend Interface

Eugen uses a generic `Backend` interface that allows swapping the LLM provider. Ollama is currently implemented; additional backends (OpenAI-compatible APIs, vLLM) are architecturally prepared.

```go
type Backend interface {
    Generate(systemPrompt, userPrompt string, onToken func(string)) (string, error)
    Chat(messages []Message, onToken func(string)) (string, error)
    Embed(text string) ([]float64, error)
    Name() string
}
```

All responses are streamed token-by-token for instant feedback in the terminal.

---

## 🔒 Privacy & Security

Eugen was built with a **privacy-first** approach:

- **No cloud API calls** – All communication goes exclusively through the local (or user-defined) Ollama endpoint
- **Air-gapped capable** – Works fully without internet access
- **No telemetry** – No tracking, no usage data, no logs sent to third parties
- **System-modifying commands require confirmation** – No command is ever auto-executed
- **Destructive commands require explicit `EXECUTE` input** – Double safety net for high-risk operations
- **Static binary** – No dynamic dependencies, no attack surface from third-party libraries

---

## 📄 License

This project is provided as open source. See the license file in the repository for details.
