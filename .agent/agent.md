# Projektrichtlinien: Eugen (SLES/openSUSE KI-Assistent)

Dieses Dokument definiert die strikten Entwicklungsregeln und Architekturvorgaben für den KI-Systemassistenten "Eugen". Programmierassistenten (wie Claude, GPT, Gemini) MÜSSEN diese Regeln bei jeder Code-Erzeugung und jedem Lösungsansatz zwingend beachten.

## 1. Zero-Bloat Philosophie & Go Standard Library
- Eugen wird **zu 100% in Go (Golang)** mit der Standardbibliothek entwickelt.
- Es dürfen **KEINE externen Go-Packages** (z. B. über `go get`) importiert werden. Weder für CLI-Parsing (kein Cobra/Viper), noch für HTTP-Anfragen (kein Resty), noch für Log-Formatierung, etc.
- Ziel ist es, ein minimalistisches, statisch kompiliertes Single-Binary (`CGO_ENABLED=0`) zu erzeugen, das sich mühelos auf Air-Gapped-Systemen verteilen lässt.

## 2. Zielplattform: SLES und openSUSE
- Die Software ist explizit und exklusiv für **SUSE Linux Enterprise Server (SLES)** und **openSUSE** vorgesehen.
- Wenn Befehle vorgeschlagen oder ausgeführt werden, nutze immer die SLES-typischen Bordmittel:
  - Paketverwaltung: `zypper` (nicht `apt` oder `yum`)
  - Dienste: `systemctl`, `journalctl`
  - Dateisysteme: Besonderer Fokus auf `btrfs` (Snapshots, Subvolumes)
- Gehe davon aus, dass SLES-Pfade (wie `/etc/os-release`) der Wahrheitstitel für die System-Erkennung sind.

## 3. Architektur & Modulstruktur
- **`cmd/eugen/main.go`**: Einstiegspunkt, CLI-Argumenten-Parsing (via `flag`) und Hauptschleife (interaktive REPL-Logik, Prompt-Gestaltung).
- **`internal/`**: Alle Kernkomponenten liegen geschützt in diesem Verzeichnis.
  - `internal/ollama/`: Simpler HTTP-Client (`net/http`, `encoding/json`) zur Kommunikation mit der lokalen Ollama-Instanz (`http://localhost:11434`).
  - `internal/executor/`: Befehlsausführung (`os/exec`) und heuristisches Risk-Scoring zur Erkennung gefährlicher Bash-Kommandos (`rm -rf`, `dd`, `mkfs`).
  - `internal/loganalyzer/`: Auslesen von Fehlerprotokollen via `journalctl -p 3` und `dmesg`.
  - `internal/context/`: Dynamisches Sammeln von Systeminformationen für Retrieval-Augmented Generation (RAG).

## 4. Datenschutz & Sicherheit (Air-Gapped)
- Es dürfen **niemals** API-Aufrufe an externe Cloud-Provider integriert werden.
- Die gesamte Kommunikation erfolgt ausschließlich über den vom User definierten (oder lokalen) Ollama-Endpunkt.
- Systemverändernde Befehle (Risk-Score > Low) erfordern stets einen expliziten **Interaktiv-Prompt** (`[J]a / [N]ein / [A]npassen` oder bei hohem Risiko das Eintippen von `EXECUTE`). Es erfolgt prinzipiell keine unkontrollierte automatische Systemveränderung.
