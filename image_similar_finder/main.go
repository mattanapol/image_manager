package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"image"
	_ "image/gif"  // Register GIF decoder
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	// External dependencies - run 'go get <path>' for these
	"github.com/corona10/goimagehash"   // Perceptual hashing
	"github.com/schollz/progressbar/v3" // Progress bar
	// Note: You might need to add more image format decoders if needed, e.g.,
	// _ "golang.org/x/image/bmp"
	// _ "golang.org/x/image/tiff"
)

// Supported image extensions (lowercase)
var imageExtensions = map[string]struct{}{
	".png":  {},
	".jpg":  {},
	".jpeg": {},
	".bmp":  {}, // Requires golang.org/x/image/bmp
	".gif":  {},
	".tiff": {}, // Requires golang.org/x/image/tiff
}

const (
	// Default hash size used by goimagehash pHash (usually 64 bits)
	defaultHashSize = 64
	// Default name for the cache file
	defaultCacheFileName = ".image_hashes.gob"
)

// ImageHashCache stores the mapping from file path to its perceptual hash.
// Using ExtImageHash as it's generally serializable.
type ImageHashCache map[string]*goimagehash.ExtImageHash

// HashJob represents a path to be processed by a worker.
type HashJob struct {
	Path string
}

// HashResult holds the result from a worker.
type HashResult struct {
	Path string
	Hash *goimagehash.ExtImageHash
	Err  error
}

// calculateHash calculates the perceptual hash for a given image file.
func calculateHash(imagePath string) (*goimagehash.ExtImageHash, error) {
	// Basic extension check first
	if !isImageExtension(imagePath) {
		return nil, nil // Not an error, just skip non-image files silently like python version
	}

	file, err := os.Open(imagePath)
	if err != nil {
		// fmt.Fprintf(os.Stderr, "Warning: Could not open file %s: %v\n", imagePath, err)
		return nil, nil // Suppress warning like python version
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		// fmt.Fprintf(os.Stderr, "Warning: Could not decode image %s: %v\n", imagePath, err)
		return nil, nil // Suppress warning like python version
	}

	// Using pHash (PerceptionHash) as in the Python example
	hash, err := goimagehash.ExtPerceptionHash(img, 8, 8) // Standard pHash parameters
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Error calculating hash for %s: %v\n", imagePath, err)
		return nil, nil // Return nil hash if calculation fails
	}

	return hash, nil
}

// isImageExtension checks if a file path has a supported image extension.
func isImageExtension(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	_, supported := imageExtensions[ext]
	return supported
}

// findImageFiles recursively finds all potential image files in the given folder.
func findImageFiles(folderPath string) ([]string, error) {
	fmt.Printf("Scanning folder: %s\n", folderPath)
	var imageFiles []string
	err := filepath.WalkDir(folderPath, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Error accessing path %q: %v\n", path, err)
			return nil
		}
		if !info.IsDir() && isImageExtension(path) {
			imageFiles = append(imageFiles, path)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking the path %q: %v", folderPath, err)
	}
	fmt.Printf("Found %d potential image files to check.\n", len(imageFiles))
	return imageFiles, nil
}

// convertPercentToDistance converts similarity percentage (100=identical) to Hamming distance threshold.
func convertPercentToDistance(percentThreshold float64, hashSize int) (int, error) {
	if percentThreshold < 0 || percentThreshold > 100 {
		return 0, fmt.Errorf("percentage threshold must be between 0 and 100, got %.2f", percentThreshold)
	}
	// Formula: distance = hash_size * (1 - percent / 100)
	// We want images with distance <= calculated max distance
	maxDistance := float64(hashSize) * (1.0 - percentThreshold/100.0)
	return int(math.Floor(maxDistance)), nil // Use floor to be inclusive
}

// calculateSimilarityPercent calculates similarity percentage from Hamming distance.
func calculateSimilarityPercent(distance int, hashSize int) float64 {
	if distance < 0 {
		distance = 0 // Should not happen with Hamming distance
	}
	if distance > hashSize {
		distance = hashSize // Cap distance at hash_size
	}

	similarity := (float64(hashSize-distance) / float64(hashSize)) * 100.0
	return similarity
}

// loadHashesFromCache loads image hashes from a cache file using gob encoding.
func loadHashesFromCache(cacheFile string) (ImageHashCache, error) {
	hashes := make(ImageHashCache)
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		fmt.Printf("Cache file %s not found, starting fresh.\n", cacheFile)
		return hashes, nil // No cache file is not an error
	}

	fmt.Printf("Loading image hashes from cache: %s\n", cacheFile)
	file, err := os.Open(cacheFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open cache file %s: %w", cacheFile, err)
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&hashes); err != nil {
		// If cache is corrupted or format changed, might be better to start fresh
		fmt.Fprintf(os.Stderr, "Warning: Could not decode cache file %s (maybe corrupted or format changed?), starting fresh: %v\n", cacheFile, err)
		return make(ImageHashCache), nil // Return empty map instead of error
	}

	return hashes, nil
}

// saveHashesToCache saves image hashes to a cache file using gob encoding.
func saveHashesToCache(cacheFile string, imageHashes ImageHashCache) error {
	file, err := os.Create(cacheFile)
	if err != nil {
		return fmt.Errorf("failed to create cache file %s: %w", cacheFile, err)
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(imageHashes); err != nil {
		// TODO: this error because goimagehash.ExtImageHash has no export value.
		return fmt.Errorf("failed to encode hashes to cache file %s: %w", cacheFile, err)
	}
	fmt.Printf("Saved %d image hashes to cache: %s\n", len(imageHashes), cacheFile)
	return nil
}

// calculateHashesParallel calculates image hashes in parallel for the given image paths using a semaphore.
func calculateHashesParallel(imagePaths []string, existingHashes ImageHashCache, numWorkers int) ImageHashCache {
	// Default to the number of logical CPUs if numWorkers is not specified or invalid.
	// This is the standard Go approach for CPU-bound tasks.
	// For I/O-bound tasks, sometimes numWorkers > runtime.NumCPU() can be beneficial,
	// but requires profiling to determine the optimal number.
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	// GOMAXPROCS defaults to runtime.NumCPU() since Go 1.5, ensuring Go can potentially
	// utilize all cores for its goroutines. You usually don't need to set this manually.
	fmt.Printf("Using %d worker goroutines for hash calculation (GOMAXPROCS=%d).\n", numWorkers, runtime.GOMAXPROCS(0))

	// Determine paths that need processing (filter out cached entries)
	pathsToProcess := make([]string, 0, len(imagePaths)) // Pre-allocate slice capacity
	for _, path := range imagePaths {
		if _, exists := existingHashes[path]; !exists {
			pathsToProcess = append(pathsToProcess, path)
		}
	}

	totalToProcess := len(pathsToProcess)
	if totalToProcess == 0 {
		fmt.Println("All image hashes found in cache. No new calculations needed.")
		return existingHashes // Return original map as no changes were made
	}

	fmt.Printf("Calculating hashes for %d new images...\n", totalToProcess)

	// Use buffered channels matching the number of jobs/results to avoid blocking
	// if channel processing is slower than job generation/completion, though
	// totalToProcess buffering might use significant memory for large inputs.
	// A smaller buffer (e.g., numWorkers * 2) is often a good compromise.
	jobs := make(chan HashJob, totalToProcess)
	results := make(chan HashResult, totalToProcess)
	var wg sync.WaitGroup

	// Start workers
	// These workers will concurrently pull paths from the 'jobs' channel,
	// process them using calculateHash, and send results to the 'results' channel.
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			// Process jobs until the 'jobs' channel is closed
			for job := range jobs {
				// *** The performance bottleneck is often HERE (in calculateHash) ***
				// It might be CPU-bound (good for this parallel model)
				// OR I/O-bound (disk read, network), which means the CPU
				// associated with this goroutine might idle while waiting.
				// Profiling (pprof) is essential to understand behavior.
				hash, err := calculateHash(job.Path)
				results <- HashResult{Path: job.Path, Hash: hash, Err: err}
			}
		}(w)
	}

	// Send jobs to the workers via the 'jobs' channel
	for _, path := range pathsToProcess {
		jobs <- HashJob{Path: path}
	}
	// Close the 'jobs' channel to signal workers that no more jobs are coming.
	// Workers currently ranging over 'jobs' will finish their current job (if any)
	// and then exit their loop upon detecting the closed channel.
	close(jobs)

	// Start a dedicated goroutine to wait for all workers to finish *before*
	// closing the results channel. This prevents a panic if we try to read
	// from 'results' after closing it but before all workers are done sending.
	go func() {
		wg.Wait()      // Wait for all goroutines launched with wg.Add(1) to call wg.Done()
		close(results) // Close the results channel *after* all workers have finished
	}()

	// Collect results
	// Using a progress bar for user feedback
	bar := progressbar.Default(int64(totalToProcess), "Hashing Images") // Use real progress bar
	newHashes := make(ImageHashCache)                                   // Store newly computed hashes here

	// Read from the 'results' channel until it's closed (by the goroutine above).
	// This loop automatically terminates when 'results' is closed and empty.
	for res := range results {
		if res.Err != nil {
			// Log error appropriately instead of just printing to stderr maybe?
			fmt.Printf("Warning: Error processing %s: %v\n", res.Path, res.Err)
			// Depending on requirements, you might want to collect errors
		} else if res.Hash != nil {
			// Store successfully computed hashes
			newHashes[res.Path] = res.Hash
		}
		// Increment progress bar for each result received (success or error)
		_ = bar.Add(1) // Ignore error for simplicity here
	}

	// It's safe to merge now because the results loop is finished, meaning all
	// workers have completed and sent their results.
	if len(newHashes) > 0 {
		fmt.Printf("\nMerging %d newly calculated hashes into the cache.\n", len(newHashes))
		for path, hash := range newHashes {
			existingHashes[path] = hash // Add new hashes to the map passed in
		}
	} else {
		fmt.Println("\nNo new hashes were successfully calculated.")
	}

	return existingHashes // Return the modified map
}

func main() {
	// --- Argument Parsing ---
	inputImage := flag.String("input", "", "Path to the input image file (required)")
	searchFolder := flag.String("folder", "", "Path to the folder to search (required)")
	threshold := flag.Float64("threshold", 90.0, "Similarity threshold percentage (0-100). Default: 90.0")
	concurrency := flag.Int("concurrency", runtime.NumCPU()-1, "Number of concurrent processes. Defaults to CPU count.")
	cacheFile := flag.String("cache", "", fmt.Sprintf("Path to the cache file. Defaults to '%s' in the search folder.", defaultCacheFileName))

	flag.Parse()

	// --- Input Validation ---
	if *inputImage == "" {
		log.Fatal("Error: Input image path (-input) is required.")
	}
	if *searchFolder == "" {
		log.Fatal("Error: Search folder path (-folder) is required.")
	}

	if info, err := os.Stat(*inputImage); err != nil || info.IsDir() {
		log.Fatalf("Error: Input image not found or is a directory: %s", *inputImage)
	}
	if info, err := os.Stat(*searchFolder); err != nil || !info.IsDir() {
		log.Fatalf("Error: Search folder not found or is not a directory: %s", *searchFolder)
	}

	distanceThreshold, err := convertPercentToDistance(*threshold, defaultHashSize)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Similarity threshold: %.2f%% translates to max Hamming distance: %d (for hash size %d)\n",
		*threshold, distanceThreshold, defaultHashSize)

	// --- Determine Cache File Path ---
	cacheFilePath := *cacheFile
	if cacheFilePath == "" {
		cacheFilePath = filepath.Join(*searchFolder, defaultCacheFileName)
	}

	// --- Load Hashes from Cache ---
	imageHashes, err := loadHashesFromCache(cacheFilePath)
	if err != nil {
		// loadHashesFromCache already prints warnings, maybe just log fatal if it's critical
		log.Printf("Warning: Proceeding without cache due to error: %v", err)
		imageHashes = make(ImageHashCache) // Ensure it's initialized
	}

	// --- Find Candidate Images ---
	candidatePaths, err := findImageFiles(*searchFolder)
	if err != nil {
		log.Fatalf("Error finding image files: %v", err)
	}
	if len(candidatePaths) == 0 {
		fmt.Println("No potential image files found in the search folder.")
		os.Exit(0)
	}

	// --- Calculate Hashes for Candidate Images (with Concurrency) ---
	if *concurrency <= 0 {
		*concurrency = 1 // Ensure at least one worker
	}
	if *concurrency == 1 {
		fmt.Println("Calculating image hashes sequentially...")
		bar := progressbar.Default(int64(len(candidatePaths)), "Hashing Images")
		for _, path := range candidatePaths {
			if _, exists := imageHashes[path]; !exists {
				hash, _ := calculateHash(path) // Ignore error like python version
				if hash != nil {
					imageHashes[path] = hash
				}
			}
			bar.Add(1)
		}
	} else {
		imageHashes = calculateHashesParallel(candidatePaths, imageHashes, *concurrency)
	}

	// --- Save Hashes to Cache ---
	if err := saveHashesToCache(cacheFilePath, imageHashes); err != nil {
		log.Printf("Warning: Could not save cache file %s: %v", cacheFilePath, err)
	}

	// --- Calculate Hash for Input Image ---
	fmt.Printf("\nCalculating hash for input image: %s\n", *inputImage)
	inputHash, err := calculateHash(*inputImage)
	if err != nil {
		// calculateHash itself might return nil error for decode/open issues
		log.Printf("Warning trying to calculate input hash: %v", err) // Log potential underlying error
	}
	if inputHash == nil {
		log.Fatalf("Error: Could not process input image: %s", *inputImage)
	}
	fmt.Printf("Input image hash: %s\n", inputHash.ToString()) // Use ToString for readable hex

	// Resolve to absolute paths to prevent matching the same file via different relative paths
	absInputImagePath, err := filepath.Abs(*inputImage)
	if err != nil {
		log.Fatalf("Error getting absolute path for input image: %v", err)
	}

	// --- Search for First Similar Image ---
	fmt.Printf("\nSearching for first similar image (Threshold >= %.2f%%)...\n", *threshold)
	foundMatch := false
	processedCount := 0

	bar := progressbar.Default(int64(len(candidatePaths)), "Scanning Candidates")

	for _, candidatePath := range candidatePaths {
		bar.Add(1)
		processedCount++

		// Avoid comparing the input file to itself using absolute paths
		absCandidatePath, err := filepath.Abs(candidatePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not get absolute path for %s: %v\n", candidatePath, err)
			continue // Skip if we can't resolve path
		}
		if absCandidatePath == absInputImagePath {
			continue
		}

		// Get hash for the candidate image from the calculated/cached hashes
		candidateHash, exists := imageHashes[candidatePath]
		if !exists || candidateHash == nil {
			continue // Skip if hash wasn't calculated or is nil
		}

		// Compare hashes using Hamming distance
		distance, err := inputHash.Distance(candidateHash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not compare hashes for %s: %v\n", candidatePath, err)
			continue // Skip if comparison fails
		}

		// Check if similarity threshold is met
		if distance <= distanceThreshold {
			similarityPercent := calculateSimilarityPercent(distance, defaultHashSize)
			// Ensure floating point inaccuracies don't show slightly below threshold
			if similarityPercent >= *threshold {
				fmt.Println("\n--- Match Found! ---")
				fmt.Printf("Input Image:    '%s'\n", *inputImage)
				fmt.Printf("Similar Image:  '%s'\n", candidatePath)
				fmt.Printf("Hamming Distance: %d (Threshold <= %d)\n", distance, distanceThreshold)
				fmt.Printf("Similarity:       %.2f%% (Threshold >= %.2f%%)\n", similarityPercent, *threshold)
				fmt.Printf("Processed %d out of %d candidates before stopping.\n", processedCount, len(candidatePaths))
				foundMatch = true
				os.Exit(0) // Stop processing like python version
			}
		}
	}

	// --- No Match Found ---
	if !foundMatch {
		fmt.Printf("\nNo similar image found matching the threshold (>= %.2f%%)\n", *threshold)
		fmt.Printf("Scanned %d candidate files.\n", processedCount)
		os.Exit(0) // Exit normally
	}
}
