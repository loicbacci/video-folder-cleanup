# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Video folder cleanup utility for Emby media libraries. Cleans up orphaned metadata folders after video files are deleted.

Expected folder structure: `library/studio/title/video.mkv`

## Build Commands

```bash
go build -o video-folder-cleanup.exe .
```

## Usage

```bash
# Dry-run (default) - shows what would be deleted
./video-folder-cleanup.exe /path/to/library1 /path/to/library2

# Actually delete folders
./video-folder-cleanup.exe --execute /path/to/library

# Adjust concurrency (default 10 workers)
./video-folder-cleanup.exe --workers 20 /path/to/library
```

## Architecture

Single-file Go program (`main.go`) with concurrent directory scanning:

- **Worker pool pattern**: Configurable number of goroutines process studio folders in parallel
- **Three-level scanning**: library → studio → title folders
- **Dry-run by default**: Requires `--execute` flag to actually delete

Video extensions recognized: `.mkv`, `.mp4`, `.avi`, `.m4v`
