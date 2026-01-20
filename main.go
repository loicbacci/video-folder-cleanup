package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var videoExtensions = map[string]bool{
	".mkv": true,
	".mp4": true,
	".avi": true,
	".m4v": true,
}

// Known metadata subdirectory suffixes that are expected in title folders
var metadataSubdirSuffixes = []string{
	".trickplay",
}

type CleanupResult struct {
	OrphanedFolders  []string // Folders with metadata but no video
	OrphanedFiles    []string // Metadata files at wrong level with no video
	EmptyFolders     []string // Completely empty folders
	StructureWarnings []string // Files/folders not matching expected structure
}

func main() {
	execute := flag.Bool("execute", false, "Actually delete folders (default is dry-run)")
	workers := flag.Int("workers", 10, "Number of concurrent workers")
	flag.Parse()

	libraryPaths := flag.Args()
	if len(libraryPaths) == 0 {
		fmt.Println("Usage: video-folder-cleanup [--execute] [--workers N] <library-path> [library-path...]")
		fmt.Println("\nOptions:")
		fmt.Println("  --execute    Actually delete folders (default is dry-run mode)")
		fmt.Println("  --workers N  Number of concurrent workers (default 10)")
		fmt.Println("\nExpected structure: library/studio/title/video.mkv")
		os.Exit(1)
	}

	if !*execute {
		fmt.Println("=== DRY RUN MODE (use --execute to actually delete) ===")
		fmt.Println()
	}

	result := &CleanupResult{}
	var resultMu sync.Mutex

	for _, libraryPath := range libraryPaths {
		fmt.Printf("Scanning library: %s\n", libraryPath)
		scanLibrary(libraryPath, *workers, result, &resultMu)
	}

	// Print results
	fmt.Println("\n" + strings.Repeat("=", 60))

	if len(result.StructureWarnings) > 0 {
		fmt.Printf("\n⚠️  Structure warnings (%d):\n", len(result.StructureWarnings))
		for _, warning := range result.StructureWarnings {
			fmt.Printf("   %s\n", warning)
		}
	}

	if len(result.OrphanedFolders) > 0 {
		fmt.Printf("\n🗑️  Orphaned metadata folders (no video file) (%d):\n", len(result.OrphanedFolders))
		for _, folder := range result.OrphanedFolders {
			fmt.Printf("   %s\n", folder)
		}
	}

	if len(result.OrphanedFiles) > 0 {
		fmt.Printf("\n🗑️  Orphaned metadata files (no video file at same level) (%d):\n", len(result.OrphanedFiles))
		for _, file := range result.OrphanedFiles {
			fmt.Printf("   %s\n", file)
		}
	}

	if len(result.EmptyFolders) > 0 {
		fmt.Printf("\n📁 Empty folders (%d):\n", len(result.EmptyFolders))
		for _, folder := range result.EmptyFolders {
			fmt.Printf("   %s\n", folder)
		}
	}

	// Execute deletions if requested
	if *execute {
		fmt.Println("\n" + strings.Repeat("=", 60))
		fmt.Println("Executing deletions...")

		deleted := 0
		failed := 0

		// Delete orphaned folders first
		for _, folder := range result.OrphanedFolders {
			if err := os.RemoveAll(folder); err != nil {
				fmt.Printf("❌ Failed to delete %s: %v\n", folder, err)
				failed++
			} else {
				fmt.Printf("✓ Deleted: %s\n", folder)
				deleted++
			}
		}

		// Delete orphaned files
		for _, file := range result.OrphanedFiles {
			if _, err := os.Stat(file); os.IsNotExist(err) {
				continue
			}
			if err := os.Remove(file); err != nil {
				fmt.Printf("❌ Failed to delete %s: %v\n", file, err)
				failed++
			} else {
				fmt.Printf("✓ Deleted: %s\n", file)
				deleted++
			}
		}

		// Delete empty folders (in reverse order to handle nested empties)
		for i := len(result.EmptyFolders) - 1; i >= 0; i-- {
			folder := result.EmptyFolders[i]
			// Check if still empty (might have been deleted as part of parent)
			if _, err := os.Stat(folder); os.IsNotExist(err) {
				continue
			}
			if err := os.Remove(folder); err != nil {
				fmt.Printf("❌ Failed to delete %s: %v\n", folder, err)
				failed++
			} else {
				fmt.Printf("✓ Deleted: %s\n", folder)
				deleted++
			}
		}

		fmt.Printf("\nDeleted %d items, %d failures\n", deleted, failed)
	} else {
		total := len(result.OrphanedFolders) + len(result.OrphanedFiles) + len(result.EmptyFolders)
		if total > 0 {
			fmt.Printf("\n💡 Run with --execute to delete %d items\n", total)
		} else {
			fmt.Println("\n✓ Nothing to clean up")
		}
	}
}

func scanLibrary(libraryPath string, numWorkers int, result *CleanupResult, resultMu *sync.Mutex) {
	// Validate library path exists
	info, err := os.Stat(libraryPath)
	if err != nil {
		fmt.Printf("Error accessing library path %s: %v\n", libraryPath, err)
		return
	}
	if !info.IsDir() {
		fmt.Printf("Library path is not a directory: %s\n", libraryPath)
		return
	}

	// Check for files directly in library (structure violation)
	checkDirectChildren(libraryPath, "library", result, resultMu)

	// Get all studio folders
	studioEntries, err := os.ReadDir(libraryPath)
	if err != nil {
		fmt.Printf("Error reading library directory %s: %v\n", libraryPath, err)
		return
	}

	// Collect studio directories
	var studioDirs []string
	for _, entry := range studioEntries {
		if entry.IsDir() {
			studioDirs = append(studioDirs, filepath.Join(libraryPath, entry.Name()))
		}
	}

	// Process studios concurrently
	studioChan := make(chan string, len(studioDirs))
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for studioPath := range studioChan {
				processStudio(studioPath, result, resultMu)
			}
		}()
	}

	for _, studioDir := range studioDirs {
		studioChan <- studioDir
	}
	close(studioChan)
	wg.Wait()

	// After processing all title folders, check for empty studio folders
	for _, studioPath := range studioDirs {
		if isEmpty, _ := isDirEmpty(studioPath); isEmpty {
			resultMu.Lock()
			result.EmptyFolders = append(result.EmptyFolders, studioPath)
			resultMu.Unlock()
		}
	}
}

func processStudio(studioPath string, result *CleanupResult, resultMu *sync.Mutex) {
	// Check for files directly in studio folder (structure violation)
	checkDirectChildren(studioPath, "studio", result, resultMu)

	// Get all title folders in this studio
	titleEntries, err := os.ReadDir(studioPath)
	if err != nil {
		resultMu.Lock()
		result.StructureWarnings = append(result.StructureWarnings,
			fmt.Sprintf("Cannot read studio directory: %s (%v)", studioPath, err))
		resultMu.Unlock()
		return
	}

	for _, entry := range titleEntries {
		if !entry.IsDir() {
			continue // Files in studio are handled by checkDirectChildren
		}

		titlePath := filepath.Join(studioPath, entry.Name())
		processTitleFolder(titlePath, result, resultMu)
	}
}

func processTitleFolder(titlePath string, result *CleanupResult, resultMu *sync.Mutex) {
	entries, err := os.ReadDir(titlePath)
	if err != nil {
		resultMu.Lock()
		result.StructureWarnings = append(result.StructureWarnings,
			fmt.Sprintf("Cannot read title directory: %s (%v)", titlePath, err))
		resultMu.Unlock()
		return
	}

	// Check if folder is empty
	if len(entries) == 0 {
		resultMu.Lock()
		result.EmptyFolders = append(result.EmptyFolders, titlePath)
		resultMu.Unlock()
		return
	}

	// Check for video files and subdirectories
	hasVideoFile := false
	var unexpectedSubdirs []string

	for _, entry := range entries {
		if entry.IsDir() {
			// Check if this is a known metadata subdirectory (e.g. movie.trickplay)
			// These are ignored - they're only valid alongside a video file
			if !isMetadataSubdir(entry.Name()) {
				unexpectedSubdirs = append(unexpectedSubdirs, entry.Name())
			}
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if videoExtensions[ext] {
			hasVideoFile = true
		}
	}

	// Warn about unexpected subdirectories in title folder
	for _, subdir := range unexpectedSubdirs {
		resultMu.Lock()
		result.StructureWarnings = append(result.StructureWarnings,
			fmt.Sprintf("Unexpected subdirectory in title folder: %s", filepath.Join(titlePath, subdir)))
		resultMu.Unlock()
	}

	// If no video file but has content (metadata files, subdirs), mark as orphaned
	if !hasVideoFile && len(entries) > 0 {
		resultMu.Lock()
		result.OrphanedFolders = append(result.OrphanedFolders, titlePath)
		resultMu.Unlock()
	}
}

func checkDirectChildren(dirPath string, level string, result *CleanupResult, resultMu *sync.Mutex) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}

	// First pass: collect all files and check for video files
	var files []string
	videoBasenames := make(map[string]bool) // basenames of video files (without extension)

	for _, entry := range entries {
		if !entry.IsDir() {
			filePath := filepath.Join(dirPath, entry.Name())
			files = append(files, filePath)

			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if videoExtensions[ext] {
				// Store the basename without extension
				basename := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
				videoBasenames[strings.ToLower(basename)] = true
			}
		}
	}

	// Second pass: categorize files
	for _, filePath := range files {
		filename := filepath.Base(filePath)
		ext := strings.ToLower(filepath.Ext(filename))

		if videoExtensions[ext] {
			// Video file at wrong level - just warn
			resultMu.Lock()
			result.StructureWarnings = append(result.StructureWarnings,
				fmt.Sprintf("Video file at %s level (should be in title folder): %s", level, filePath))
			resultMu.Unlock()
		} else {
			// Non-video file - check if it's orphaned metadata
			basename := strings.TrimSuffix(filename, ext)
			// Check if there's a video with matching basename prefix
			// e.g., "movie.nfo" matches "movie.mkv", "movie-poster.jpg" matches "movie.mkv"
			hasMatchingVideo := false
			for videoBase := range videoBasenames {
				if strings.HasPrefix(strings.ToLower(basename), videoBase) {
					hasMatchingVideo = true
					break
				}
			}

			if hasMatchingVideo {
				// Metadata file with matching video - just warn about location
				resultMu.Lock()
				result.StructureWarnings = append(result.StructureWarnings,
					fmt.Sprintf("Metadata file at %s level (should be in title folder): %s", level, filePath))
				resultMu.Unlock()
			} else {
				// Orphaned metadata file - no matching video
				resultMu.Lock()
				result.OrphanedFiles = append(result.OrphanedFiles, filePath)
				resultMu.Unlock()
			}
		}
	}
}

func isDirEmpty(dirPath string) (bool, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func isMetadataSubdir(name string) bool {
	for _, suffix := range metadataSubdirSuffixes {
		if strings.HasSuffix(strings.ToLower(name), suffix) {
			return true
		}
	}
	return false
}
