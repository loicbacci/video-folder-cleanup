package main

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
)

// Helper function to create a test directory structure
func setupTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "video-cleanup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	return dir
}

// Helper to create a file
func createFile(t *testing.T, path string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("Failed to create directory %s: %v", dir, err)
	}
	if err := os.WriteFile(path, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create file %s: %v", path, err)
	}
}

// Helper to create a directory
func createDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("Failed to create directory %s: %v", path, err)
	}
}

// ============================================================================
// Tests for videoExtensions map
// ============================================================================

func TestVideoExtensions(t *testing.T) {
	tests := []struct {
		ext      string
		expected bool
	}{
		{".mkv", true},
		{".mp4", true},
		{".avi", true},
		{".m4v", true},
		{".txt", false},
		{".nfo", false},
		{".jpg", false},
		{".srt", false},
		{".MKV", false}, // Case sensitive - extensions should be lowercased before lookup
		{"mkv", false},  // Missing dot
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.ext, func(t *testing.T) {
			result := videoExtensions[tc.ext]
			if result != tc.expected {
				t.Errorf("videoExtensions[%q] = %v, want %v", tc.ext, result, tc.expected)
			}
		})
	}
}

// ============================================================================
// Tests for isDirEmpty
// ============================================================================

func TestIsDirEmpty_EmptyDirectory(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	emptyDir := filepath.Join(tempDir, "empty")
	createDir(t, emptyDir)

	isEmpty, err := isDirEmpty(emptyDir)
	if err != nil {
		t.Fatalf("isDirEmpty returned error: %v", err)
	}
	if !isEmpty {
		t.Error("isDirEmpty should return true for empty directory")
	}
}

func TestIsDirEmpty_NonEmptyDirectory(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	nonEmptyDir := filepath.Join(tempDir, "nonempty")
	createFile(t, filepath.Join(nonEmptyDir, "file.txt"))

	isEmpty, err := isDirEmpty(nonEmptyDir)
	if err != nil {
		t.Fatalf("isDirEmpty returned error: %v", err)
	}
	if isEmpty {
		t.Error("isDirEmpty should return false for non-empty directory")
	}
}

func TestIsDirEmpty_DirectoryWithSubdir(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	parentDir := filepath.Join(tempDir, "parent")
	createDir(t, filepath.Join(parentDir, "child"))

	isEmpty, err := isDirEmpty(parentDir)
	if err != nil {
		t.Fatalf("isDirEmpty returned error: %v", err)
	}
	if isEmpty {
		t.Error("isDirEmpty should return false for directory with subdirectory")
	}
}

func TestIsDirEmpty_NonExistentDirectory(t *testing.T) {
	_, err := isDirEmpty("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("isDirEmpty should return error for non-existent directory")
	}
}

// ============================================================================
// Tests for checkDirectChildren
// ============================================================================

func TestCheckDirectChildren_NoFiles(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	// Create directory with only subdirectories
	createDir(t, filepath.Join(tempDir, "subdir1"))
	createDir(t, filepath.Join(tempDir, "subdir2"))

	result := &CleanupResult{}
	var mu sync.Mutex
	checkDirectChildren(tempDir, "library", result, &mu)

	if len(result.StructureWarnings) != 0 {
		t.Errorf("Expected 0 warnings, got %d: %v", len(result.StructureWarnings), result.StructureWarnings)
	}
}

func TestCheckDirectChildren_WithFiles(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	// Create directory with files (structure violation) - no matching video
	createFile(t, filepath.Join(tempDir, "file1.txt"))
	createFile(t, filepath.Join(tempDir, "file2.nfo"))
	createDir(t, filepath.Join(tempDir, "subdir"))

	result := &CleanupResult{}
	var mu sync.Mutex
	checkDirectChildren(tempDir, "library", result, &mu)

	// Files without matching video are orphaned files, not warnings
	if len(result.OrphanedFiles) != 2 {
		t.Errorf("Expected 2 orphaned files for files at library level, got %d", len(result.OrphanedFiles))
	}
}

func TestCheckDirectChildren_WithVideoAndMetadata(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	// Create video file and matching metadata at library level
	createFile(t, filepath.Join(tempDir, "movie.mkv"))
	createFile(t, filepath.Join(tempDir, "movie.nfo"))
	createFile(t, filepath.Join(tempDir, "movie-poster.jpg"))

	result := &CleanupResult{}
	var mu sync.Mutex
	checkDirectChildren(tempDir, "library", result, &mu)

	// Video and its metadata at wrong level generate warnings (not orphaned)
	if len(result.StructureWarnings) != 3 {
		t.Errorf("Expected 3 warnings for video+metadata at library level, got %d: %v",
			len(result.StructureWarnings), result.StructureWarnings)
	}
	if len(result.OrphanedFiles) != 0 {
		t.Errorf("Expected 0 orphaned files (video present), got %d", len(result.OrphanedFiles))
	}
}

func TestCheckDirectChildren_OrphanedMetadata(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	// Create metadata files with no matching video (orphaned)
	createFile(t, filepath.Join(tempDir, "deleted-movie.nfo"))
	createFile(t, filepath.Join(tempDir, "deleted-movie-poster.jpg"))

	result := &CleanupResult{}
	var mu sync.Mutex
	checkDirectChildren(tempDir, "library", result, &mu)

	// Metadata without matching video are orphaned
	if len(result.OrphanedFiles) != 2 {
		t.Errorf("Expected 2 orphaned files, got %d", len(result.OrphanedFiles))
	}
	if len(result.StructureWarnings) != 0 {
		t.Errorf("Expected 0 warnings, got %d", len(result.StructureWarnings))
	}
}

func TestCheckDirectChildren_MixedOrphanedAndMatching(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	// Mix of: video+metadata (warnings) and orphaned metadata (orphaned files)
	createFile(t, filepath.Join(tempDir, "existing.mkv"))
	createFile(t, filepath.Join(tempDir, "existing.nfo"))      // matches video
	createFile(t, filepath.Join(tempDir, "deleted.nfo"))       // orphaned
	createFile(t, filepath.Join(tempDir, "deleted-poster.jpg")) // orphaned

	result := &CleanupResult{}
	var mu sync.Mutex
	checkDirectChildren(tempDir, "library", result, &mu)

	// existing.mkv and existing.nfo generate warnings
	if len(result.StructureWarnings) != 2 {
		t.Errorf("Expected 2 warnings, got %d: %v", len(result.StructureWarnings), result.StructureWarnings)
	}
	// deleted.nfo and deleted-poster.jpg are orphaned
	if len(result.OrphanedFiles) != 2 {
		t.Errorf("Expected 2 orphaned files, got %d: %v", len(result.OrphanedFiles), result.OrphanedFiles)
	}
}

func TestCheckDirectChildren_NonExistentDir(t *testing.T) {
	result := &CleanupResult{}
	var mu sync.Mutex
	checkDirectChildren("/nonexistent/path", "library", result, &mu)

	// Should not panic and should not add warnings for non-existent dir
	if len(result.StructureWarnings) != 0 {
		t.Errorf("Expected 0 warnings for non-existent dir, got %d", len(result.StructureWarnings))
	}
}

// ============================================================================
// Tests for processTitleFolder
// ============================================================================

func TestProcessTitleFolder_WithVideoFile(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	titleDir := filepath.Join(tempDir, "title")
	createFile(t, filepath.Join(titleDir, "movie.mkv"))
	createFile(t, filepath.Join(titleDir, "movie.nfo"))
	createFile(t, filepath.Join(titleDir, "poster.jpg"))

	result := &CleanupResult{}
	var mu sync.Mutex
	processTitleFolder(titleDir, result, &mu)

	if len(result.OrphanedFolders) != 0 {
		t.Errorf("Expected no orphaned folders, got %d", len(result.OrphanedFolders))
	}
	if len(result.EmptyFolders) != 0 {
		t.Errorf("Expected no empty folders, got %d", len(result.EmptyFolders))
	}
	if len(result.StructureWarnings) != 0 {
		t.Errorf("Expected no warnings, got %d", len(result.StructureWarnings))
	}
}

func TestProcessTitleFolder_OrphanedMetadata(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	titleDir := filepath.Join(tempDir, "title")
	// Create metadata files but no video file
	createFile(t, filepath.Join(titleDir, "movie.nfo"))
	createFile(t, filepath.Join(titleDir, "poster.jpg"))
	createFile(t, filepath.Join(titleDir, "fanart.jpg"))

	result := &CleanupResult{}
	var mu sync.Mutex
	processTitleFolder(titleDir, result, &mu)

	if len(result.OrphanedFolders) != 1 {
		t.Errorf("Expected 1 orphaned folder, got %d", len(result.OrphanedFolders))
	}
	if len(result.OrphanedFolders) > 0 && result.OrphanedFolders[0] != titleDir {
		t.Errorf("Orphaned folder path mismatch: got %s, want %s", result.OrphanedFolders[0], titleDir)
	}
}

func TestProcessTitleFolder_Empty(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	titleDir := filepath.Join(tempDir, "title")
	createDir(t, titleDir)

	result := &CleanupResult{}
	var mu sync.Mutex
	processTitleFolder(titleDir, result, &mu)

	if len(result.EmptyFolders) != 1 {
		t.Errorf("Expected 1 empty folder, got %d", len(result.EmptyFolders))
	}
	if len(result.OrphanedFolders) != 0 {
		t.Errorf("Expected no orphaned folders, got %d", len(result.OrphanedFolders))
	}
}

func TestProcessTitleFolder_WithSubdirectory(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	titleDir := filepath.Join(tempDir, "title")
	createFile(t, filepath.Join(titleDir, "movie.mkv"))
	createDir(t, filepath.Join(titleDir, "extras"))

	result := &CleanupResult{}
	var mu sync.Mutex
	processTitleFolder(titleDir, result, &mu)

	if len(result.StructureWarnings) != 1 {
		t.Errorf("Expected 1 warning for subdirectory, got %d", len(result.StructureWarnings))
	}
}

func TestProcessTitleFolder_WithTrickplaySubdirectory(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	titleDir := filepath.Join(tempDir, "title")
	createFile(t, filepath.Join(titleDir, "movie.mkv"))
	createDir(t, filepath.Join(titleDir, "movie.trickplay"))

	result := &CleanupResult{}
	var mu sync.Mutex
	processTitleFolder(titleDir, result, &mu)

	if len(result.StructureWarnings) != 0 {
		t.Errorf("Expected no warnings for .trickplay subdirectory, got %d: %v",
			len(result.StructureWarnings), result.StructureWarnings)
	}
}

func TestProcessTitleFolder_MixedSubdirectories(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	titleDir := filepath.Join(tempDir, "title")
	createFile(t, filepath.Join(titleDir, "movie.mkv"))
	createDir(t, filepath.Join(titleDir, "movie.trickplay")) // Expected metadata subdir
	createDir(t, filepath.Join(titleDir, "extras"))          // Unexpected subdir
	createDir(t, filepath.Join(titleDir, "featurettes"))     // Unexpected subdir

	result := &CleanupResult{}
	var mu sync.Mutex
	processTitleFolder(titleDir, result, &mu)

	if len(result.StructureWarnings) != 2 {
		t.Errorf("Expected 2 warnings for unexpected subdirectories, got %d: %v",
			len(result.StructureWarnings), result.StructureWarnings)
	}
}

func TestProcessTitleFolder_OnlyTrickplayNoVideo(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	titleDir := filepath.Join(tempDir, "title")
	// Only .trickplay folder, no video - should be orphaned
	createDir(t, filepath.Join(titleDir, "movie.trickplay"))

	result := &CleanupResult{}
	var mu sync.Mutex
	processTitleFolder(titleDir, result, &mu)

	if len(result.StructureWarnings) != 0 {
		t.Errorf("Expected no warnings for .trickplay, got %d: %v",
			len(result.StructureWarnings), result.StructureWarnings)
	}
	if len(result.OrphanedFolders) != 1 {
		t.Errorf("Expected folder with only .trickplay to be orphaned, got %d orphaned",
			len(result.OrphanedFolders))
	}
}

func TestProcessTitleFolder_TrickplayWithMetadataNoVideo(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	titleDir := filepath.Join(tempDir, "title")
	// .trickplay folder + metadata files but no video - should be orphaned
	createDir(t, filepath.Join(titleDir, "movie.trickplay"))
	createFile(t, filepath.Join(titleDir, "movie.nfo"))
	createFile(t, filepath.Join(titleDir, "poster.jpg"))

	result := &CleanupResult{}
	var mu sync.Mutex
	processTitleFolder(titleDir, result, &mu)

	if len(result.StructureWarnings) != 0 {
		t.Errorf("Expected no warnings, got %d: %v",
			len(result.StructureWarnings), result.StructureWarnings)
	}
	if len(result.OrphanedFolders) != 1 {
		t.Errorf("Expected folder to be orphaned, got %d orphaned",
			len(result.OrphanedFolders))
	}
}

func TestProcessTitleFolder_AllVideoFormats(t *testing.T) {
	formats := []string{".mkv", ".mp4", ".avi", ".m4v"}

	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			tempDir := setupTestDir(t)
			defer os.RemoveAll(tempDir)

			titleDir := filepath.Join(tempDir, "title")
			createFile(t, filepath.Join(titleDir, "movie"+format))

			result := &CleanupResult{}
			var mu sync.Mutex
			processTitleFolder(titleDir, result, &mu)

			if len(result.OrphanedFolders) != 0 {
				t.Errorf("Video format %s should be recognized, but folder was marked orphaned", format)
			}
		})
	}
}

func TestProcessTitleFolder_CaseInsensitiveExtension(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	titleDir := filepath.Join(tempDir, "title")
	createFile(t, filepath.Join(titleDir, "movie.MKV")) // Uppercase extension

	result := &CleanupResult{}
	var mu sync.Mutex
	processTitleFolder(titleDir, result, &mu)

	if len(result.OrphanedFolders) != 0 {
		t.Error("Uppercase video extension should be recognized")
	}
}

func TestProcessTitleFolder_MixedCaseExtension(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	titleDir := filepath.Join(tempDir, "title")
	createFile(t, filepath.Join(titleDir, "movie.Mkv")) // Mixed case

	result := &CleanupResult{}
	var mu sync.Mutex
	processTitleFolder(titleDir, result, &mu)

	if len(result.OrphanedFolders) != 0 {
		t.Error("Mixed case video extension should be recognized")
	}
}

// ============================================================================
// Tests for processStudio
// ============================================================================

func TestProcessStudio_ValidStructure(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	studioDir := filepath.Join(tempDir, "Studio A")
	createFile(t, filepath.Join(studioDir, "Movie 1", "movie.mkv"))
	createFile(t, filepath.Join(studioDir, "Movie 2", "movie.mp4"))

	result := &CleanupResult{}
	var mu sync.Mutex
	processStudio(studioDir, result, &mu)

	if len(result.OrphanedFolders) != 0 {
		t.Errorf("Expected no orphaned folders, got %d", len(result.OrphanedFolders))
	}
	if len(result.StructureWarnings) != 0 {
		t.Errorf("Expected no warnings, got %d: %v", len(result.StructureWarnings), result.StructureWarnings)
	}
}

func TestProcessStudio_WithFilesAtStudioLevel(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	studioDir := filepath.Join(tempDir, "Studio A")
	createFile(t, filepath.Join(studioDir, "Movie 1", "movie.mkv"))
	createFile(t, filepath.Join(studioDir, "random.txt")) // File at studio level (no matching video)

	result := &CleanupResult{}
	var mu sync.Mutex
	processStudio(studioDir, result, &mu)

	// File without matching video is orphaned
	if len(result.OrphanedFiles) != 1 {
		t.Errorf("Expected 1 orphaned file at studio level, got %d", len(result.OrphanedFiles))
	}
}

func TestProcessStudio_MixedContent(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	studioDir := filepath.Join(tempDir, "Studio A")
	// Valid title with video
	createFile(t, filepath.Join(studioDir, "Movie 1", "movie.mkv"))
	// Orphaned title (no video)
	createFile(t, filepath.Join(studioDir, "Movie 2", "movie.nfo"))
	// Empty title
	createDir(t, filepath.Join(studioDir, "Movie 3"))

	result := &CleanupResult{}
	var mu sync.Mutex
	processStudio(studioDir, result, &mu)

	if len(result.OrphanedFolders) != 1 {
		t.Errorf("Expected 1 orphaned folder, got %d", len(result.OrphanedFolders))
	}
	if len(result.EmptyFolders) != 1 {
		t.Errorf("Expected 1 empty folder, got %d", len(result.EmptyFolders))
	}
}

// ============================================================================
// Tests for scanLibrary
// ============================================================================

func TestScanLibrary_CompleteStructure(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	libraryDir := filepath.Join(tempDir, "Library")

	// Studio 1 with valid movies
	createFile(t, filepath.Join(libraryDir, "Studio1", "Movie1", "movie.mkv"))
	createFile(t, filepath.Join(libraryDir, "Studio1", "Movie1", "movie.nfo"))
	createFile(t, filepath.Join(libraryDir, "Studio1", "Movie2", "movie.mp4"))

	// Studio 2 with orphaned folder
	createFile(t, filepath.Join(libraryDir, "Studio2", "Movie3", "movie.avi"))
	createFile(t, filepath.Join(libraryDir, "Studio2", "OrphanedMovie", "poster.jpg"))

	result := &CleanupResult{}
	var mu sync.Mutex
	scanLibrary(libraryDir, 4, result, &mu)

	if len(result.OrphanedFolders) != 1 {
		t.Errorf("Expected 1 orphaned folder, got %d", len(result.OrphanedFolders))
	}
}

func TestScanLibrary_EmptyStudios(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	libraryDir := filepath.Join(tempDir, "Library")
	createDir(t, filepath.Join(libraryDir, "EmptyStudio"))
	createFile(t, filepath.Join(libraryDir, "Studio1", "Movie1", "movie.mkv"))

	result := &CleanupResult{}
	var mu sync.Mutex
	scanLibrary(libraryDir, 4, result, &mu)

	if len(result.EmptyFolders) != 1 {
		t.Errorf("Expected 1 empty folder (empty studio), got %d", len(result.EmptyFolders))
	}
}

func TestScanLibrary_FilesAtLibraryLevel(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	libraryDir := filepath.Join(tempDir, "Library")
	createFile(t, filepath.Join(libraryDir, "readme.txt")) // No matching video
	createDir(t, filepath.Join(libraryDir, "Studio1"))

	result := &CleanupResult{}
	var mu sync.Mutex
	scanLibrary(libraryDir, 4, result, &mu)

	// File without matching video is orphaned
	if len(result.OrphanedFiles) != 1 {
		t.Errorf("Expected 1 orphaned file at library level, got %d", len(result.OrphanedFiles))
	}
}

func TestScanLibrary_NonExistentPath(t *testing.T) {
	result := &CleanupResult{}
	var mu sync.Mutex

	// Should not panic
	scanLibrary("/nonexistent/path/library", 4, result, &mu)

	// No crashes means success
}

func TestScanLibrary_FileInsteadOfDirectory(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "notadirectory.txt")
	createFile(t, filePath)

	result := &CleanupResult{}
	var mu sync.Mutex

	// Should not panic when given a file instead of directory
	scanLibrary(filePath, 4, result, &mu)
}

func TestScanLibrary_ConcurrencyStress(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	libraryDir := filepath.Join(tempDir, "Library")

	// Create many studios and titles to stress test concurrency
	for i := 0; i < 20; i++ {
		for j := 0; j < 10; j++ {
			titleDir := filepath.Join(libraryDir,
				"Studio"+string(rune('A'+i)),
				"Movie"+string(rune('0'+j)))
			if j%3 == 0 {
				// Orphaned folder
				createFile(t, filepath.Join(titleDir, "metadata.nfo"))
			} else if j%3 == 1 {
				// Valid folder with video
				createFile(t, filepath.Join(titleDir, "video.mkv"))
			} else {
				// Empty folder
				createDir(t, titleDir)
			}
		}
	}

	result := &CleanupResult{}
	var mu sync.Mutex

	// Test with different worker counts
	for _, workers := range []int{1, 4, 10, 20, 50} {
		result = &CleanupResult{}
		scanLibrary(libraryDir, workers, result, &mu)

		// Should have consistent results regardless of worker count
		expectedOrphaned := 20 * 4  // 4 orphaned per studio (j % 3 == 0 for j=0,3,6,9)
		expectedEmpty := 20 * 3     // 3 empty per studio (j % 3 == 2 for j=2,5,8)

		if len(result.OrphanedFolders) != expectedOrphaned {
			t.Errorf("Workers=%d: Expected %d orphaned folders, got %d",
				workers, expectedOrphaned, len(result.OrphanedFolders))
		}
		if len(result.EmptyFolders) != expectedEmpty {
			t.Errorf("Workers=%d: Expected %d empty folders, got %d",
				workers, expectedEmpty, len(result.EmptyFolders))
		}
	}
}

// ============================================================================
// Integration-style tests
// ============================================================================

func TestIntegration_RealisticLibraryStructure(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	libraryDir := filepath.Join(tempDir, "Movies")

	// Warner Bros studio
	createFile(t, filepath.Join(libraryDir, "Warner Bros", "The Matrix (1999)", "The Matrix.mkv"))
	createFile(t, filepath.Join(libraryDir, "Warner Bros", "The Matrix (1999)", "The Matrix.nfo"))
	createFile(t, filepath.Join(libraryDir, "Warner Bros", "The Matrix (1999)", "poster.jpg"))
	createFile(t, filepath.Join(libraryDir, "Warner Bros", "The Matrix (1999)", "fanart.jpg"))

	// Deleted movie - only metadata remains
	createFile(t, filepath.Join(libraryDir, "Warner Bros", "Deleted Movie (2020)", "Deleted Movie.nfo"))
	createFile(t, filepath.Join(libraryDir, "Warner Bros", "Deleted Movie (2020)", "poster.jpg"))

	// Universal studio
	createFile(t, filepath.Join(libraryDir, "Universal", "Jurassic Park (1993)", "Jurassic Park.mp4"))
	createFile(t, filepath.Join(libraryDir, "Universal", "Jurassic Park (1993)", "movie.nfo"))

	// Empty folder where movie was completely removed
	createDir(t, filepath.Join(libraryDir, "Universal", "Gone Movie (2021)"))

	// Empty studio
	createDir(t, filepath.Join(libraryDir, "Empty Studio"))

	result := &CleanupResult{}
	var mu sync.Mutex
	scanLibrary(libraryDir, 4, result, &mu)

	// Verify orphaned folders
	if len(result.OrphanedFolders) != 1 {
		t.Errorf("Expected 1 orphaned folder, got %d: %v", len(result.OrphanedFolders), result.OrphanedFolders)
	}

	// Verify empty folders (title folder + empty studio)
	if len(result.EmptyFolders) != 2 {
		t.Errorf("Expected 2 empty folders, got %d: %v", len(result.EmptyFolders), result.EmptyFolders)
	}

	// Verify no structure warnings (everything follows expected structure)
	if len(result.StructureWarnings) != 0 {
		t.Errorf("Expected 0 structure warnings, got %d: %v", len(result.StructureWarnings), result.StructureWarnings)
	}
}

func TestIntegration_MultipleLibraries(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	// Create two libraries
	library1 := filepath.Join(tempDir, "Movies")
	library2 := filepath.Join(tempDir, "TV Shows")

	createFile(t, filepath.Join(library1, "Studio1", "Movie1", "movie.mkv"))
	createFile(t, filepath.Join(library1, "Studio1", "OrphanedMovie", "poster.jpg"))

	createFile(t, filepath.Join(library2, "Network1", "Show1", "show.mp4"))
	createDir(t, filepath.Join(library2, "Network1", "EmptyShow"))

	result := &CleanupResult{}
	var mu sync.Mutex

	scanLibrary(library1, 4, result, &mu)
	scanLibrary(library2, 4, result, &mu)

	if len(result.OrphanedFolders) != 1 {
		t.Errorf("Expected 1 orphaned folder across libraries, got %d", len(result.OrphanedFolders))
	}
	if len(result.EmptyFolders) != 1 {
		t.Errorf("Expected 1 empty folder across libraries, got %d", len(result.EmptyFolders))
	}
}

// ============================================================================
// Edge case tests
// ============================================================================

func TestEdgeCase_SpecialCharactersInNames(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	libraryDir := filepath.Join(tempDir, "Library")

	// Folders with special characters
	createFile(t, filepath.Join(libraryDir, "Studio's Name", "Movie & Title (2020)", "movie.mkv"))
	createFile(t, filepath.Join(libraryDir, "Studio [HD]", "Movie - Part 1", "orphaned.nfo"))

	result := &CleanupResult{}
	var mu sync.Mutex
	scanLibrary(libraryDir, 4, result, &mu)

	if len(result.OrphanedFolders) != 1 {
		t.Errorf("Expected 1 orphaned folder with special chars, got %d", len(result.OrphanedFolders))
	}
}

func TestEdgeCase_DeepNestedSubdirectories(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	libraryDir := filepath.Join(tempDir, "Library")
	titleDir := filepath.Join(libraryDir, "Studio", "Title")

	createFile(t, filepath.Join(titleDir, "movie.mkv"))
	// Create unexpected deep nesting
	createFile(t, filepath.Join(titleDir, "extras", "behind_scenes", "video.mp4"))

	result := &CleanupResult{}
	var mu sync.Mutex
	scanLibrary(libraryDir, 4, result, &mu)

	// Should warn about subdirectory in title folder
	if len(result.StructureWarnings) != 1 {
		t.Errorf("Expected 1 warning for nested subdirectory, got %d: %v",
			len(result.StructureWarnings), result.StructureWarnings)
	}
}

func TestEdgeCase_OnlyHiddenFiles(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	libraryDir := filepath.Join(tempDir, "Library")
	titleDir := filepath.Join(libraryDir, "Studio", "Title")

	// Create only hidden files (Unix-style, may not be hidden on Windows)
	createFile(t, filepath.Join(titleDir, ".DS_Store"))
	createFile(t, filepath.Join(titleDir, ".nfo"))

	result := &CleanupResult{}
	var mu sync.Mutex
	scanLibrary(libraryDir, 4, result, &mu)

	// Hidden files are still files, so this should be orphaned (no video)
	if len(result.OrphanedFolders) != 1 {
		t.Errorf("Expected 1 orphaned folder with only hidden files, got %d", len(result.OrphanedFolders))
	}
}

func TestEdgeCase_VideoFileWithMetadata(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	libraryDir := filepath.Join(tempDir, "Library")
	titleDir := filepath.Join(libraryDir, "Studio", "Title")

	// Video file with lots of metadata files
	createFile(t, filepath.Join(titleDir, "movie.mkv"))
	createFile(t, filepath.Join(titleDir, "movie.nfo"))
	createFile(t, filepath.Join(titleDir, "movie-poster.jpg"))
	createFile(t, filepath.Join(titleDir, "movie-fanart.jpg"))
	createFile(t, filepath.Join(titleDir, "movie-banner.jpg"))
	createFile(t, filepath.Join(titleDir, "movie.srt"))
	createFile(t, filepath.Join(titleDir, "movie.en.srt"))

	result := &CleanupResult{}
	var mu sync.Mutex
	scanLibrary(libraryDir, 4, result, &mu)

	if len(result.OrphanedFolders) != 0 {
		t.Error("Folder with video and metadata should not be orphaned")
	}
	if len(result.EmptyFolders) != 0 {
		t.Error("Folder with video should not be empty")
	}
}

func TestEdgeCase_MultipleVideoFiles(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	libraryDir := filepath.Join(tempDir, "Library")
	titleDir := filepath.Join(libraryDir, "Studio", "Title")

	// Multiple video files in same folder
	createFile(t, filepath.Join(titleDir, "movie-cd1.avi"))
	createFile(t, filepath.Join(titleDir, "movie-cd2.avi"))

	result := &CleanupResult{}
	var mu sync.Mutex
	scanLibrary(libraryDir, 4, result, &mu)

	if len(result.OrphanedFolders) != 0 {
		t.Error("Folder with multiple video files should not be orphaned")
	}
}

func TestEdgeCase_ZeroWorkers(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	libraryDir := filepath.Join(tempDir, "Library")
	createFile(t, filepath.Join(libraryDir, "Studio", "Title", "movie.mkv"))

	result := &CleanupResult{}
	var mu sync.Mutex

	// Zero workers should effectively do nothing (no goroutines started)
	// This tests that the code handles edge case gracefully
	scanLibrary(libraryDir, 0, result, &mu)

	// With 0 workers, studios won't be processed, but we should not crash
}

// ============================================================================
// Test CleanupResult sorting for predictability
// ============================================================================

func TestCleanupResult_Sorting(t *testing.T) {
	tempDir := setupTestDir(t)
	defer os.RemoveAll(tempDir)

	libraryDir := filepath.Join(tempDir, "Library")

	// Create folders that would be processed in unpredictable order
	createFile(t, filepath.Join(libraryDir, "Zebra Studio", "Movie", "orphan.nfo"))
	createFile(t, filepath.Join(libraryDir, "Alpha Studio", "Movie", "orphan.nfo"))
	createFile(t, filepath.Join(libraryDir, "Middle Studio", "Movie", "orphan.nfo"))

	result := &CleanupResult{}
	var mu sync.Mutex
	scanLibrary(libraryDir, 4, result, &mu)

	if len(result.OrphanedFolders) != 3 {
		t.Fatalf("Expected 3 orphaned folders, got %d", len(result.OrphanedFolders))
	}

	// Sort for predictable comparison
	sort.Strings(result.OrphanedFolders)

	if !containsSubstring(result.OrphanedFolders[0], "Alpha Studio") {
		t.Errorf("First sorted folder should be Alpha Studio, got %s", result.OrphanedFolders[0])
	}
}

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
// Benchmark tests
// ============================================================================

func BenchmarkScanLibrary_Small(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	libraryDir := filepath.Join(tempDir, "Library")
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			path := filepath.Join(libraryDir, "Studio"+string(rune('A'+i)), "Movie"+string(rune('0'+j)), "movie.mkv")
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				b.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
				b.Fatal(err)
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := &CleanupResult{}
		var mu sync.Mutex
		scanLibrary(libraryDir, 10, result, &mu)
	}
}

func BenchmarkScanLibrary_ConcurrencyComparison(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	libraryDir := filepath.Join(tempDir, "Library")
	for i := 0; i < 50; i++ {
		for j := 0; j < 20; j++ {
			path := filepath.Join(libraryDir, "Studio"+string(rune('A'+i%26))+string(rune('0'+i/26)),
				"Movie"+string(rune('0'+j%10))+string(rune('0'+j/10)), "movie.mkv")
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				b.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
				b.Fatal(err)
			}
		}
	}

	workerCounts := []int{1, 4, 10, 20}
	for _, workers := range workerCounts {
		b.Run("workers="+string(rune('0'+workers%10)), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				result := &CleanupResult{}
				var mu sync.Mutex
				scanLibrary(libraryDir, workers, result, &mu)
			}
		})
	}
}
