package materialize

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cosmscm/cosm/pkg/core"
)

// VFSFile represents an in-memory file descriptor within the MemoryVFS.
type VFSFile struct {
	path    string
	data    []byte
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
}

// Name returns the base name of the file.
func (f *VFSFile) Name() string {
	return path.Base(f.path)
}

// Size returns the file size in bytes.
func (f *VFSFile) Size() int64 {
	return int64(len(f.data))
}

// Mode returns the file mode.
func (f *VFSFile) Mode() fs.FileMode {
	return f.mode
}

// ModTime returns the modification time.
func (f *VFSFile) ModTime() time.Time {
	return f.modTime
}

// IsDir returns whether the entry is a directory.
func (f *VFSFile) IsDir() bool {
	return f.isDir
}

// Sys returns underlying raw source (nil).
func (f *VFSFile) Sys() any {
	return nil
}

// Type returns the file mode type bits.
func (f *VFSFile) Type() fs.FileMode {
	return f.mode.Type()
}

// Info returns the fs.FileInfo representation.
func (f *VFSFile) Info() (fs.FileInfo, error) {
	return f, nil
}

// memFileHandle implements fs.File and io.ReadSeekCloser for open virtual files.
type memFileHandle struct {
	file   *VFSFile
	reader *bytes.Reader
}

func (h *memFileHandle) Stat() (fs.FileInfo, error) {
	return h.file, nil
}

func (h *memFileHandle) Read(b []byte) (int, error) {
	if h.reader == nil {
		return 0, io.EOF
	}
	return h.reader.Read(b)
}

func (h *memFileHandle) Seek(offset int64, whence int) (int64, error) {
	if h.reader == nil {
		return 0, io.EOF
	}
	return h.reader.Seek(offset, whence)
}

func (h *memFileHandle) Close() error {
	return nil
}

// MemoryVFS is a high-performance in-memory virtual filesystem implementing fs.FS, fs.ReadFileFS, fs.ReadDirFS, fs.StatFS.
type MemoryVFS struct {
	mu    sync.RWMutex
	files map[string]*VFSFile
	dirs  map[string]bool
}

// NewMemoryVFS initializes an empty in-memory VFS.
func NewMemoryVFS() *MemoryVFS {
	vfs := &MemoryVFS{
		files: make(map[string]*VFSFile),
		dirs:  make(map[string]bool),
	}
	vfs.dirs["."] = true
	return vfs
}

// cleanPath standardizes path separators to '/' and trims leading slashes or dots.
func cleanPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = path.Clean(p)
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "."
	}
	return p
}

// WriteFile stores or overwrites a file in the virtual filesystem.
func (v *MemoryVFS) WriteFile(filePath string, data []byte, perm os.FileMode) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	p := cleanPath(filePath)
	if p == "." {
		return fmt.Errorf("cannot write file to root")
	}

	// Register parent directories
	dir := path.Dir(p)
	for dir != "." && dir != "/" && dir != "" {
		v.dirs[dir] = true
		dir = path.Dir(dir)
	}

	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	v.files[p] = &VFSFile{
		path:    p,
		data:    dataCopy,
		mode:    perm,
		modTime: time.Now().UTC(),
		isDir:   false,
	}

	return nil
}

// ReadFile returns the byte content of a virtual file.
func (v *MemoryVFS) ReadFile(name string) ([]byte, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	p := cleanPath(name)
	f, ok := v.files[p]
	if !ok || f.isDir {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrNotExist}
	}

	dataCopy := make([]byte, len(f.data))
	copy(dataCopy, f.data)
	return dataCopy, nil
}

// Open opens the named file for reading (implements fs.FS).
func (v *MemoryVFS) Open(name string) (fs.File, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	p := cleanPath(name)
	if p == "." || v.dirs[p] {
		return &memFileHandle{
			file: &VFSFile{
				path:    p,
				mode:    os.ModeDir | 0755,
				modTime: time.Now().UTC(),
				isDir:   true,
			},
			reader: nil,
		}, nil
	}

	f, ok := v.files[p]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	return &memFileHandle{
		file:   f,
		reader: bytes.NewReader(f.data),
	}, nil
}

// Stat returns FileInfo describing the named file.
func (v *MemoryVFS) Stat(name string) (fs.FileInfo, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	p := cleanPath(name)
	if p == "." || v.dirs[p] {
		return &VFSFile{
			path:    p,
			mode:    os.ModeDir | 0755,
			modTime: time.Now().UTC(),
			isDir:   true,
		}, nil
	}

	f, ok := v.files[p]
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}

	return f, nil
}

// ReadDir reads the named directory and returns a list of directory entries sorted by name.
func (v *MemoryVFS) ReadDir(name string) ([]fs.DirEntry, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	p := cleanPath(name)
	if p != "." && !v.dirs[p] {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}

	seenEntries := make(map[string]fs.DirEntry)

	// Collect files directly under p
	for fPath, f := range v.files {
		dir := path.Dir(fPath)
		if dir == p || (p == "." && !strings.Contains(fPath, "/")) {
			seenEntries[f.Name()] = f
		}
	}

	// Collect immediate subdirectories under p
	for dPath := range v.dirs {
		if dPath == p || dPath == "." {
			continue
		}
		parent := path.Dir(dPath)
		if parent == p || (p == "." && !strings.Contains(dPath, "/")) {
			base := path.Base(dPath)
			seenEntries[base] = &VFSFile{
				path:    dPath,
				mode:    os.ModeDir | 0755,
				modTime: time.Now().UTC(),
				isDir:   true,
			}
		}
	}

	var entries []fs.DirEntry
	for _, e := range seenEntries {
		entries = append(entries, e)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	return entries, nil
}

// ListFiles returns all relative file paths currently stored in the VFS.
func (v *MemoryVFS) ListFiles() []string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	var paths []string
	for p := range v.files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// Remove deletes a file or directory from the VFS.
func (v *MemoryVFS) Remove(name string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()

	p := cleanPath(name)
	delete(v.files, p)
	delete(v.dirs, p)
	return true
}

// ToMap returns a snapshot map of all file paths to their byte contents.
func (v *MemoryVFS) ToMap() map[string][]byte {
	v.mu.RLock()
	defer v.mu.RUnlock()

	res := make(map[string][]byte, len(v.files))
	for p, f := range v.files {
		dataCopy := make([]byte, len(f.data))
		copy(dataCopy, f.data)
		res[p] = dataCopy
	}
	return res
}

// FromMap populates the MemoryVFS from a map of file paths to content bytes.
func (v *MemoryVFS) FromMap(files map[string][]byte) {
	for path, data := range files {
		_ = v.WriteFile(path, data, 0644)
	}
}

// HydrateToVFS hydrates a full WorkspaceManifestNode into a MemoryVFS instance.
func HydrateToVFS(
	manifest *core.WorkspaceManifestNode,
	compMap map[string]*core.ComponentNode,
	symbolMap map[string]*core.ASTSymbolNode,
) (*MemoryVFS, error) {
	hydrator := NewHydrator()
	fileMap, err := hydrator.HydrateWorkspace(manifest, compMap, symbolMap)
	if err != nil {
		return nil, fmt.Errorf("hydration failed: %w", err)
	}

	vfs := NewMemoryVFS()
	vfs.FromMap(fileMap)
	return vfs, nil
}
