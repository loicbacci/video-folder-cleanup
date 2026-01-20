# video-folder-cleanup

> Built with [Claude Code](https://claude.ai/code)

A utility for cleaning up orphaned metadata folders and files in Emby media libraries after video files are deleted.

## Features

- **Concurrent scanning** - Configurable worker pool for fast processing of large libraries
- **Orphaned folder detection** - Finds title folders containing metadata but no video file
- **Orphaned file detection** - Finds metadata files at wrong directory levels with no matching video
- **Empty folder detection** - Finds completely empty folders
- **Dry-run by default** - See what would be deleted before committing
- **Metadata-aware** - Recognizes `.trickplay` subdirectories as valid metadata

## Installation

### From releases

Download the latest binary for your platform from the [releases page](https://github.com/loicbacci/video-folder-cleanup/releases).

### From source

```bash
go install github.com/loicbacci/video-folder-cleanup@latest
```

Or clone and build:

```bash
git clone https://github.com/loicbacci/video-folder-cleanup.git
cd video-folder-cleanup
go build -o video-folder-cleanup .
```

## Usage

### Expected folder structure

```text
library/
  Studio A/
    Movie Title (2020)/
      movie.mkv
      movie.nfo
      poster.jpg
      movie.trickplay/
    Another Movie (2021)/
      video.mp4
  Studio B/
    ...
```

### Commands

```bash
# Dry-run (default) - shows what would be deleted
./video-folder-cleanup /path/to/library

# Scan multiple libraries
./video-folder-cleanup /path/to/movies /path/to/tv-shows

# Actually delete folders and files
./video-folder-cleanup --execute /path/to/library

# Adjust concurrency (default 10 workers)
./video-folder-cleanup --workers 20 /path/to/library
```

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `--execute` | `false` | Actually delete folders and files (default is dry-run) |
| `--workers` | `10` | Number of concurrent workers for scanning |

## What gets detected

### Orphaned metadata folders

Title folders that contain metadata files (`.nfo`, images, `.trickplay` folders) but no video file. These typically occur when you delete a video but Emby's metadata remains.

### Orphaned metadata files

Metadata files found at the library or studio level (wrong location) that don't have a matching video file at the same level. For example, `movie.nfo` without a corresponding `movie.mkv`.

### Empty folders

Completely empty title or studio folders.

### Structure warnings

Files or folders in unexpected locations that won't be automatically deleted:
- Video files at library/studio level (should be in title folders)
- Metadata files with matching video at wrong level
- Unexpected subdirectories in title folders

## Supported video formats

- `.mkv`
- `.mp4`
- `.avi`
- `.m4v`

## License

MIT
