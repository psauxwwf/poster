# poster

`poster` is a small Go CLI that runs a NotebookLM pipeline for one or more source URLs and saves three artifacts locally:

- Markdown report: `.md`
- Infographic image: `.png`
- Audio summary: `.mp3`

It can also run as a Telegram bot that accepts `/run` commands from a single admin user.

## What It Is For

This repository exists to automate a repeatable content pipeline around NotebookLM:

1. create a notebook
2. add one or more source URLs
3. wait for indexing
4. generate a report
5. generate an infographic
6. generate an audio summary
7. download all artifacts to disk

The same pipeline can be used from the CLI or through Telegram bot mode.

## Supported Sources

`poster` accepts one or more URLs supported by NotebookLM.

Examples:

- YouTube links
- GitHub links
- other web URLs that NotebookLM can ingest

Multiple URLs are passed as separate command-line arguments.

## Prerequisites

- Go `1.26.2`
- Python `3.12.13`
- `uv`
- `notebooklm` CLI installed and available in `PATH`
- NotebookLM authentication completed with `notebooklm login`

Python dependency is managed through `uv`:

```bash
uv sync
```

## Build

Build the local binary:

```bash
task build
```

The binary will be created at:

```bash
./bin/poster
```

## CLI Usage

Always run CLI commands with `--print-logs`.

Run with one URL:

```bash
./bin/poster --print-logs "https://example.com/source"
```

Run with several URLs:

```bash
./bin/poster --print-logs \
  "https://example.com/source-1" \
  "https://github.com/example/repo" \
  "https://example.com/source-3"
```

Custom output directory:

```bash
./bin/poster --print-logs --out ./dist/notebooklm "https://example.com/source"
```

Increase timeouts:

```bash
./bin/poster --print-logs --timeout-source 15m --timeout-artifact 25m "https://example.com/source"
```

Write JSON logs to a file too:

```bash
./bin/poster --print-logs --log-file ./dist/poster.log "https://example.com/source"
```

Debug logging:

```bash
./bin/poster --print-logs --log-level debug "https://example.com/source"
```

General form:

```bash
./bin/poster --print-logs [flags] <url> [url...]
```

## CLI Output

On success, the command prints paths for:

- `image: ...png`
- `report: ...md`
- `audio: ...mp3`

By default files are written into `./dist`.

## Telegram Bot Mode

`poster` can run as a Telegram bot in `--serve` mode.

Required environment variables:

- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_ADMIN_ID`

The application loads `.env` automatically if it exists.

Example `.env`:

```env
TELEGRAM_BOT_TOKEN=123456:example
TELEGRAM_ADMIN_ID=123456789
```

Run the bot:

```bash
./bin/poster --serve --print-logs
```

Supported Telegram commands:

- `/start`
- `/run <url>`

Only the configured admin user is allowed to run commands.

## Cleanup

Delete all available notebooks:

```bash
./bin/poster --delete-all
```

## Docker

Build and start with Docker Compose:

```bash
task docker:up
```

The compose setup mounts:

- `~/.notebooklm` into the container for NotebookLM auth state
- `./dist` for generated artifacts

Container entrypoint defaults to:

```bash
./poster --serve --print-logs
```

## Notes

- `poster` sets NotebookLM language to `ru` during initialization.
- Empty URLs are ignored.
- Filenames are sanitized before writing.
- If a target filename already exists, a numeric suffix is added.
- Some transient NotebookLM failures are retried automatically.

## Troubleshooting

If a run fails, check:

- `notebooklm` is installed and callable
- `notebooklm login` has been completed
- each URL is valid and supported by NotebookLM
- timeouts are large enough for indexing and artifact generation

Useful first debugging command:

```bash
./bin/poster --print-logs --log-level debug "https://example.com/source"
```
