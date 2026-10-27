// Package envfile reads private literal KEY=value files without changing the
// process environment or evaluating shell syntax.
package envfile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
)

const maxFileBytes = 1 << 20

// File owns private value buffers. Callers must clear their Lookup copies when
// they are finished; Close clears only the buffers owned by File.
type File struct {
	mu     sync.RWMutex
	values map[string][]byte
}

// Load accepts single-line assignments, blank lines and whole-line # comments.
// Values are literal: quotes delimit a value but never enable escaping,
// interpolation, command substitution or inline comments. Whitespace and #
// inside a value require matching single or double quotes. An empty path returns
// an empty File without filesystem access. Missing explicit paths are errors.
func Load(path string) (*File, error) {
	result := &File{values: make(map[string][]byte)}
	if path == "" {
		return result, nil
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect env file: %w", err)
	}
	if err := validateFile(before); err != nil {
		return nil, err
	}
	// Match the repository's credential-file policy. NONBLOCK also prevents a
	// concurrently substituted FIFO from blocking before we can inspect it.
	descriptor, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open env file without following links: %w", err)
	}
	file := os.NewFile(uintptr(descriptor), path)
	if file == nil {
		_ = syscall.Close(descriptor)
		return nil, errors.New("open env file: invalid descriptor")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect opened env file: %w", err)
	}
	if err := validateFile(opened); err != nil {
		return nil, err
	}
	if !os.SameFile(before, opened) {
		return nil, errors.New("env file changed while opening")
	}
	// One bounded allocation avoids leaving secret copies in grown read buffers.
	content := make([]byte, int(opened.Size())+1)
	defer clear(content)
	read, err := io.ReadFull(file, content)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("read env file: %w", err)
	}
	if int64(read) != opened.Size() {
		return nil, errors.New("env file size changed while reading")
	}
	content = content[:read]
	for index, line := range bytes.Split(content, []byte{'\n'}) {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		line = bytes.TrimLeft(line, " \t")
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		key, raw, found := bytes.Cut(line, []byte{'='})
		if !found || !validName(key) {
			result.Close()
			return nil, fmt.Errorf("env file line %d: expected KEY=value with a valid variable name", index+1)
		}
		if _, exists := result.values[string(key)]; exists {
			result.Close()
			return nil, fmt.Errorf("env file line %d: duplicate key", index+1)
		}
		value, err := literalValue(raw)
		if err != nil {
			result.Close()
			return nil, fmt.Errorf("env file line %d: %w", index+1, err)
		}
		result.values[string(key)] = bytes.Clone(value)
	}
	return result, nil
}

func validateFile(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return errors.New("env file must be regular and must not be a symbolic link")
	}
	if info.Mode().Perm()&0o077 != 0 || info.Mode().Perm()&0o400 == 0 {
		return errors.New("env file requires owner read access and no group or other permissions (0400 or 0600)")
	}
	if info.Size() < 0 || info.Size() > maxFileBytes {
		return errors.New("env file exceeds the 1 MiB bound")
	}
	return nil
}

func validName(name []byte) bool {
	if len(name) == 0 {
		return false
	}
	for index, char := range name {
		if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || index > 0 && char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

func literalValue(raw []byte) ([]byte, error) {
	if bytes.ContainsAny(raw, "\x00\r\n") {
		return nil, errors.New("NUL and multiline values are unsupported")
	}
	if len(raw) > 0 && (raw[0] == '\'' || raw[0] == '"') {
		quote := raw[0]
		if len(raw) < 2 || raw[len(raw)-1] != quote || bytes.IndexByte(raw[1:len(raw)-1], quote) >= 0 {
			return nil, errors.New("quoted values must end on the same line; quote escapes and trailing text are unsupported")
		}
		return raw[1 : len(raw)-1], nil
	}
	if bytes.ContainsAny(raw, " \t\v\f#'\"") {
		return nil, errors.New("values containing whitespace, # or quotes must be quoted")
	}
	return raw, nil
}

// Lookup never falls back to the process environment. Empty values are present
// and return true; absent names and closed Files return false.
func (file *File) Lookup(name string) ([]byte, bool) {
	if file == nil {
		return nil, false
	}
	file.mu.RLock()
	defer file.mu.RUnlock()
	value, exists := file.values[name]
	return bytes.Clone(value), exists
}

// Close is idempotent and can run concurrently with Lookup.
func (file *File) Close() {
	if file == nil {
		return
	}
	file.mu.Lock()
	defer file.mu.Unlock()
	for _, value := range file.values {
		clear(value[:cap(value)])
	}
	file.values = nil
}
