# mastosync

> Disclaimer: Please use this small script at your own risk. I wrote it up quickly in one afternoon and make no guarantees to its
> usefulness, correctness etc.

# Introduction

_mastosync_ is a small command-line utility that reads RSS feeds and toots items from those feeds to a specified Mastodon instance or Bluesky.
Functionality is similar to [mastofeed](https://mastofeed.org/) but as a CLI instead of a comfortable web service.

# Installation

Ensure you have Go installed, then clone the repository and build:

```sh
go build -o mastosync
```

# Global Flags

- `--config <path>`: Path to a directory for configuration and databases. Defaults to `.mastosync` in the current directory or `~/.mastosync`.

# Commands

### `init` (alias: `i`)
Sets up the sync directory with default templates and a skeleton configuration file.

```sh
mastosync init
```

### `sync` (alias: `s`)
Syncs RSS feeds with Mastodon or Bluesky.

```sh
mastosync sync [flags]
```
**Flags:**
- `--dryrun`: Perform a dry run without actually posting.
- `--sky`: Sync to Bluesky instead of Mastodon.

### `catchup` (alias: `c`)
Records all existing items in the RSS feeds as already processed without posting them. Useful when first setting up.

```sh
mastosync catchup [flags]
```
**Flags:**
- `--sky`: Catchup for Bluesky feeds.

### `save` (alias: `n`)
Saves a Mastodon status or Bluesky skeet (and its thread) to a Notion page or a local directory as a Markdown file.

```sh
mastosync save [flags] <status-id-or-url>
```
**Flags:**
- `--dryrun`: Dry run the save operation.
- `--debug`: Enable debug logging and print the Notion API request.
- `--external`: (Notion only) Do not use Google Drive to store media; rely on the external Mastodon server URLs.
- `--title <string>`: Specify a custom title for the Notion page or the Markdown file.
- `--dir <path>`: Save the status to a local directory as an Obsidian-compatible Markdown file.

**Local Markdown Export:**
When the `--dir` flag is provided, _mastosync_ will:
1. Create a Markdown file in the specified directory with Obsidian-compatible YAML frontmatter.
2. Include platform-specific metadata (date, author, URL) and automatically add a `mastodon` or `bluesky` tag along with any hashtags from the post.
3. Download all media attachments into an `images/` subfolder.
4. Convert images in uncommon formats (like `.jfif`) to standard `.jpg`.
5. Rename image files to their SHA-256 hash to ensure uniqueness and avoid duplicates.
6. Use Obsidian-style `![[images/hash.png]]` links within the Markdown file.

### `chain` (alias: `x`)
Posts a chain of toots from a text file. It automatically splits sentences into individual toots if necessary.

```sh
mastosync chain [flags] --toots <path-to-file>
```
**Flags:**
- `--toots <path>`: **(Required)** Path to a `.txt` file containing the text to be posted as a chain.
- `--dryrun`: Dry run the posting.

### `auth` (alias: `a`)
Refreshes the OAuth token for Google Drive integration. It opens a browser for authentication.

```sh
mastosync auth
```

### `mandala` (alias: `m`)
Posts a "mandala" (image) to Mastodon and Bluesky.

```sh
mastosync mandala [flags]
```
**Flags:**
- `--path <path>`: Path to the directory containing mandalas (defaults to `/tmp`).
- `--toot <text>`: Optional extra text to include with the post.

# Configuration

The configuration is stored in `config.yaml` within your config directory.

### `config.yaml` Fields

| Field | Description |
|-------|-------------|
| `mas` | Mastodon client configuration (`server`, `clientid`, `clientsecret`, `accesstoken`). |
| `feeds` | List of `{feedurl, template}` pairs for Mastodon syncing. |
| `skyfeeds` | List of `{feedurl, template}` pairs for Bluesky syncing. |
| `notiontoken` | Your Notion Integration Token. |
| `notionparent` | The ID of the parent Notion page where saved toots will be created. |
| `bluesky` | Bluesky credentials (`handle`, `apikey`). |
| `bridge` | URL prefix for the Google Drive media bridge. |
| `parent` | Google Drive folder ID for storing media. |
| `mandala` | Path to the mandala generation script. |

### Templates

Templates are stored in the `templates/` (for Mastodon) and `skytemplates/` (for Bluesky) directories. They use Go's `text/template` syntax.

Available variables in templates:
- `{{.Title}}`: The title of the RSS item.
- `{{.Description}}`: The description/content of the RSS item.
- `{{.Link}}`: The URL of the RSS item.

Example `templates/default.tmpl`:
```
{{.Title}}

{{.Description}}

{{.Link}}
```

