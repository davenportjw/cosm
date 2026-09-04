package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

var validHashRegex = regexp.MustCompile(`^[a-f0-9]{64}$`)

// BlobStore provides disk-backed, immutable, content-addressed storage for AST objects.
type BlobStore struct {
	objectsDir string
	tmpDir     string
	mu         sync.RWMutex
}

// NewBlobStore initializes a content-addressed blob store within the specified objects directory.
// If rootPath points to a repo or .cosm directory, it creates the objects and .tmp subdirectories.
func NewBlobStore(rootPath string) (*BlobStore, error) {
	var objectsDir string
	if filepath.Base(rootPath) == "objects" {
		objectsDir = rootPath
	} else if filepath.Base(rootPath) == ".cosm" || filepath.Base(rootPath) == ".fg" {
		objectsDir = filepath.Join(rootPath, "objects")
	} else {
		if _, err := os.Stat(filepath.Join(rootPath, ".fg", "objects")); err == nil {
			objectsDir = filepath.Join(rootPath, ".fg", "objects")
		} else {
			objectsDir = filepath.Join(rootPath, ".cosm", "objects")
		}
	}

	tmpDir := filepath.Join(objectsDir, ".tmp")

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create blobstore directory %s: %w", tmpDir, err)
	}

	return &BlobStore{
		objectsDir: objectsDir,
		tmpDir:     tmpDir,
	}, nil
}

// ObjectsDir returns the root directory where blob objects are stored.
func (b *BlobStore) ObjectsDir() string {
	return b.objectsDir
}

// objectPath computes the sharded file path for a 64-character SHA-256 hash.
// E.g., objects/4f/8a2e...bin
func (b *BlobStore) objectPath(hash string) (string, error) {
	if !validHashRegex.MatchString(hash) {
		return "", fmt.Errorf("invalid sha256 hash format: %q", hash)
	}
	prefix := hash[:2]
	rest := hash[2:] + ".bin"
	return filepath.Join(b.objectsDir, prefix, rest), nil
}

// Put writes data to the content-addressed store atomically and returns its SHA-256 hash.
// If an identical blob already exists, it verifies its integrity and returns immediately.
func (b *BlobStore) Put(data []byte) (string, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	targetPath, err := b.objectPath(hash)
	if err != nil {
		return "", err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if already exists and is non-empty
	if info, err := os.Stat(targetPath); err == nil && info.Size() == int64(len(data)) {
		return hash, nil
	}

	// Ensure parent shard directory exists
	shardDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(shardDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create shard directory %s: %w", shardDir, err)
	}

	// Generate a unique temp file name
	var randBytes [8]byte
	if _, err := rand.Read(randBytes[:]); err != nil {
		return "", fmt.Errorf("failed to generate random suffix: %w", err)
	}
	tmpFileName := fmt.Sprintf("%s_%s.tmp", hash, hex.EncodeToString(randBytes[:]))
	tmpFilePath := filepath.Join(b.tmpDir, tmpFileName)

	// Write payload with fsync
	tmpFile, err := os.OpenFile(tmpFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file %s: %w", tmpFilePath, err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFilePath)
		return "", fmt.Errorf("failed to write data to temp file: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFilePath)
		return "", fmt.Errorf("failed to fsync temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpFilePath)
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename to final path
	if err := os.Rename(tmpFilePath, targetPath); err != nil {
		_ = os.Remove(tmpFilePath)
		return "", fmt.Errorf("failed to atomically rename blob to %s: %w", targetPath, err)
	}

	// Sync parent shard directory descriptor to ensure directory metadata persistence
	if shardDirFile, err := os.Open(shardDir); err == nil {
		_ = shardDirFile.Sync()
		_ = shardDirFile.Close()
	}

	return hash, nil
}

// PutWithHash verifies that the data matches the expected SHA-256 hash and writes it.
func (b *BlobStore) PutWithHash(expectedHash string, data []byte) error {
	sum := sha256.Sum256(data)
	actualHash := hex.EncodeToString(sum[:])
	if actualHash != expectedHash {
		return fmt.Errorf("hash mismatch: expected %s, computed %s", expectedHash, actualHash)
	}
	_, err := b.Put(data)
	return err
}

// Get retrieves the raw blob content for a given SHA-256 hash and validates its integrity.
func (b *BlobStore) Get(hash string) ([]byte, error) {
	targetPath, err := b.objectPath(hash)
	if err != nil {
		return nil, err
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	data, err := os.ReadFile(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("blob not found: %s", hash)
		}
		return nil, fmt.Errorf("failed to read blob %s: %w", hash, err)
	}

	// SHA-256 integrity verification against bit rot
	sum := sha256.Sum256(data)
	computedHash := hex.EncodeToString(sum[:])
	if computedHash != hash {
		return nil, fmt.Errorf("blob corruption detected for %s: computed hash is %s", hash, computedHash)
	}

	return data, nil
}

// Has checks whether a blob exists in the store.
func (b *BlobStore) Has(hash string) (bool, error) {
	targetPath, err := b.objectPath(hash)
	if err != nil {
		return false, err
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	_, err = os.Stat(targetPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// Delete removes a blob from the store.
func (b *BlobStore) Delete(hash string) error {
	targetPath, err := b.objectPath(hash)
	if err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete blob %s: %w", hash, err)
	}
	return nil
}

// List iterates through the store and returns all stored object hashes.
func (b *BlobStore) List() ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var hashes []string
	entries, err := os.ReadDir(b.objectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read objects dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".tmp" || len(entry.Name()) != 2 {
			continue
		}
		prefix := entry.Name()
		shardPath := filepath.Join(b.objectsDir, prefix)
		shardEntries, err := os.ReadDir(shardPath)
		if err != nil {
			continue
		}
		for _, se := range shardEntries {
			if se.IsDir() {
				continue
			}
			name := se.Name()
			if filepath.Ext(name) == ".bin" {
				rest := name[:len(name)-4]
				hash := prefix + rest
				if validHashRegex.MatchString(hash) {
					hashes = append(hashes, hash)
				}
			}
		}
	}

	return hashes, nil
}

// VerifyIntegrity scans all objects in the store and returns a slice of corrupted object hashes.
func (b *BlobStore) VerifyIntegrity() ([]string, error) {
	hashes, err := b.List()
	if err != nil {
		return nil, err
	}

	var corrupted []string
	for _, hash := range hashes {
		data, err := b.Get(hash)
		if err != nil {
			corrupted = append(corrupted, hash)
			continue
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != hash {
			corrupted = append(corrupted, hash)
		}
	}

	return corrupted, nil
}

// Prune removes all blobs that are not present in the keepHashes set.
// It returns the number of deleted blobs.
func (b *BlobStore) Prune(keepHashes map[string]bool) (int, error) {
	hashes, err := b.List()
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, hash := range hashes {
		if !keepHashes[hash] {
			if err := b.Delete(hash); err != nil {
				return deleted, err
			}
			deleted++
		}
	}

	return deleted, nil
}

// Size returns the total byte size of all stored objects.
func (b *BlobStore) Size() (int64, error) {
	hashes, err := b.List()
	if err != nil {
		return 0, err
	}

	var totalSize int64
	for _, hash := range hashes {
		targetPath, err := b.objectPath(hash)
		if err != nil {
			continue
		}
		info, err := os.Stat(targetPath)
		if err == nil {
			totalSize += info.Size()
		}
	}

	return totalSize, nil
}
