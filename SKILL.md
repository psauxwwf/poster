---
name: poster-cli
description: Use when the user wants to run `poster` from the command line to generate a Markdown report, infographic image, and audio summary from one or more source URLs through NotebookLM.
metadata:
tags: "poster, cli, notebooklm, youtube, report, infographic, audio, markdown, automation, go, command-line"
category: "automation"
license: MIT
---

# Poster CLI

Use this skill when the goal is to run `poster` directly from the terminal, without Telegram bot mode.

Always run `poster` with `--print-logs` so execution details are visible in stderr during the full NotebookLM pipeline.

`poster` is a small Go CLI that orchestrates a NotebookLM pipeline for source URLs and saves three local artifacts:

- Markdown report: `.md`
- Infographic image: `.png`
- Audio summary: `.mp3`

The CLI accepts one or more source URLs and passes them to NotebookLM. These can be YouTube links, GitHub links, and other URLs supported by NotebookLM.

## When To Use

Use this skill when the user wants to:

- generate artifacts from a URL in the local shell
- generate artifacts from several URLs in one command
- run the NotebookLM flow without Telegram
- save outputs into `./dist` or a custom directory
- troubleshoot `poster` CLI execution
- clean up notebooks created by previous runs

Do not use this skill when the user wants Telegram bot operation. That is `poster --serve`, which is a different workflow.

## What The CLI Does

For a normal CLI run, `poster` performs this pipeline:

1. Verifies the `notebooklm` CLI is available and authenticated.
2. Creates a temporary notebook.
3. Adds every provided URL as a source.
4. Waits until NotebookLM finishes indexing the sources.
5. Generates a Markdown report.
6. Generates an infographic.
7. Generates a short audio summary.
8. Renames the notebook based on generated content.
9. Downloads artifacts to the output directory.

Saved files use the notebook title as the base filename and default to `./dist`.

## Prerequisites

Before running `poster`, ensure all of the following are true:

- Go toolchain is installed if building locally.
- Python `3.12.13` is available.
- `uv` environment is installed from `pyproject.toml` and `uv.lock`.
- `notebooklm` CLI is installed and on `PATH`.
- NotebookLM is already authenticated with `notebooklm login`.

Recommended local setup:

```bash
uv sync
notebooklm login
```

If the local binary is used from the repo, the main executable is:

```bash
./bin/poster
```

## Primary Commands

Run a normal pipeline with one URL:

```bash
./bin/poster --print-logs "https://example.com/source"
```

Run one pipeline with several URLs separated by spaces:

```bash
./bin/poster --print-logs "https://example.com/source-1" "https://github.com/example/repo" "https://example.com/source-3"
```

Run with visible logs:

```bash
./bin/poster --print-logs "https://example.com/source"
```

Write structured logs to a file:

```bash
./bin/poster --print-logs --log-file ./dist/poster.log "https://example.com/source"
```

Choose a custom output directory:

```bash
./bin/poster --print-logs --out ./dist/notebooklm "https://example.com/source"
```

Tune timeouts for slow NotebookLM operations:

```bash
./bin/poster --print-logs --timeout-source 15m --timeout-artifact 25m "https://example.com/source"
```

Delete all available notebooks and exit:

```bash
./bin/poster --delete-all
```

## CLI Flags

Important flags:

- `--print-logs`: always enable this for CLI runs so progress and failures are visible in stderr
- `--out`: output directory for generated files, default `./dist`
- `--timeout-source`: wait time for source indexing, default `10m`
- `--timeout-artifact`: wait time for report, image, and audio generation, default `15m`
- `--notebooklm-bin`: custom path to the `notebooklm` executable
- `--log-level`: `debug`, `info`, `warn`, or `error`
- `--log-file`: optional JSON log file path
- `--print-logs`: prints logs to stderr
- `--delete-all`: deletes all notebooks and exits

For CLI usage, the main invocation shape is:

```bash
poster --print-logs [flags] <url> [url...]
```

## Outputs

A successful run prints paths for:

- `image: ...png`
- `report: ...md`
- `audio: ...mp3`

The report is downloaded first and then source links are appended to the bottom when source URLs are available.

## Operational Notes

- `poster` sets NotebookLM language to `ru` during initialization.
- Empty or whitespace-only URLs are ignored before execution.
- Several URLs can be passed in one command as separate arguments.
- The tool retries some transient NotebookLM failures such as `502`, `503`, `504`, timeouts, connection resets, and `EOF`.
- Filenames are sanitized before being written to disk.
- If a filename already exists, `poster` adds a numeric suffix.

## Troubleshooting

If a run fails, check these first:

- `notebooklm` is installed and accessible in `PATH`
- `notebooklm login` has already been completed
- every provided URL is valid and supported by NotebookLM
- source indexing has not exceeded `--timeout-source`
- artifact generation has not exceeded `--timeout-artifact`

Useful debugging command:

```bash
./bin/poster --print-logs --log-level debug "https://example.com/source"
```

If NotebookLM is flaky, prefer increasing timeouts before changing code.

## Expected Agent Behavior

When using this skill, the agent should:

1. Verify whether the user wants local CLI execution rather than Telegram mode.
2. Always include `--print-logs` in CLI runs.
3. Build with `task build` if the binary is missing or stale.
4. Use `./bin/poster` for repo-local runs unless the user requests another path.
5. Pass multiple URLs as separate CLI arguments when the user provides several sources.
6. Surface artifact paths clearly after a successful run.
7. Use `--print-logs` or `--log-file` when diagnosing failures.
8. Use `--delete-all` only when the user explicitly wants cleanup.

## Quality Checklist

Before considering the task complete, verify:

1. The binary exists or was built successfully.
2. `notebooklm` is callable.
3. The command uses the correct URL set and includes `--print-logs`.
4. The run completed without NotebookLM timeout or auth errors.
5. Expected artifact files exist in the output directory.
6. Report, image, and audio paths were surfaced to the user.
