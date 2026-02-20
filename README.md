![mastosync Hero](mastosync_hero.png)

# 🐘🦋 mastosync

**mastosync** is a sleek, powerful Go-based command-line utility designed to bridge your RSS feeds with the decentralized social web. Whether you're broadcasting to **Mastodon** or **Bluesky**, *mastosync* makes automated cross-posting effortless, reliable, and highly customizable.

> [!CAUTION]
> **Use at your own risk.** This tool was crafted with speed and specific needs in mind. While it works reliably for many, always verify your configuration before running it in production environments.

---

## ✨ Features

- **🔄 Multi-Platform Syncing**: Seamlessly post from RSS feeds to both Mastodon and Bluesky.
- **📥 Smart Saving**: Capture Mastodon toots or Bluesky skeets (including full threads) directly into **Notion** or local **Obsidian-ready Markdown**.
- **🖼️ Media Handling**:
    - Automatic image downloading and SHA-256 deduplication.
    - Intelligent format conversion (e.g., `.jfif` → `.jpg`).
    - integration with **Google Drive** for high-quality media bridging.
- **🎨 Mandala Integration**: Built-in support for posting generated mandalas.
- **⛓️ Thread Support**: Automatically split long text files into a coherent chain of posts.
- **🤖 MCP Server Mode**: Full support for the **Model Context Protocol**, allowing AI agents (like Gemini or Claude) to use *mastosync* as a specialized toolset.
- **🛠️ Template Engine**: Fully customizable post formatting using Go's `text/template` syntax.

---

## 🚀 Usage

### ⚙️ Installation

#### Build from Source
```bash
go build -o mastosync
```

---

## 📋 Commands

### `init` (alias: `i`)
Initialize your workspace with default templates and a configuration skeleton.
```bash
mastosync init
```

### `sync` (alias: `s`)
Synchronize your RSS feeds with your social profiles.
```bash
mastosync sync [--dryrun] [--sky]
```

### `save` (alias: `n`)
Archive a post or thread to Notion or Obsidian.
```bash
mastosync save [--dir <path>] [--title <string>] <status-id-or-url>
```

### `chain` (alias: `x`)
Post a sequence of updates from a text file.
```bash
mastosync chain --toots <path-to-file> [--dryrun]
```

### `mcp`
Launch *mastosync* as an MCP server.
```bash
mastosync mcp
```

---

## 🛠️ Configuration & Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--config` | Path to configuration directory | `~/.mastosync` |

### `config.yaml` Structure

| Field | Description |
|-------|-------------|
| `mas` | Mastodon API credentials (`server`, `clientid`, etc.) |
| `skyfeeds` | Configuration for Bluesky RSS synchronization |
| `bluesky` | Bluesky account credentials (`handle`, `apikey`) |
| `notiontoken` | Your Notion integration secret |
| `parent` | Google Drive folder ID for media storage |

---

## 🏗️ How it Works

1.  **RSS Ingestion**: Periodically polls configured feeds for new entries.
2.  **State Management**: Uses a local database to ensure items are never posted twice.
3.  **Template Rendering**: Merges RSS data (`Title`, `Description`, `Link`) with your custom `.tmpl` files.
4.  **Media Processing**: Downloads, hashes, and prepares attachments for the target platform.
5.  **API Dispatch**: Communicates via OAuth to Mastodon or via AT Protocol to Bluesky.

---

## 🔮 Future Proposals

We're constantly thinking about how to make *mastosync* better. Here are some ideas on the horizon:

- **🧠 AI Summaries**: Use LLMs to condense long articles into snappy social media updates.
- **🌉 Cross-Platform Bridging**: Automatically sync replies and likes across platforms.
- **📅 Visual Scheduler**: A dashboard-like view to schedule posts and manage feeds.
- **🔍 Deep Search**: Full-text search across your archived social media interactions.

---

## 📄 License

Distributed under the [MIT](LICENSE) License.

