package catalog

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
)

// Digest hashes ordered relative paths and contents using length prefixes.
// Sources must be snapshotted before applying a later installation transaction.
func Digest(source fs.FS) (string, error) {
	if err := checkTree(source); err != nil {
		return "", err
	}
	h := sha256.New()
	var total int64
	count := 0
	err := fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link not allowed: %s", name)
		}
		file, err := source.Open(name)
		if err != nil {
			return err
		}
		contents, readErr := io.ReadAll(io.LimitReader(file, 1024*1024+1))
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		count++
		total += int64(len(contents))
		if len(contents) > 1024*1024 || count > 512 || total > 10*1024*1024 {
			return fmt.Errorf("package size limit exceeded: %s", name)
		}
		writePart(h, []byte(name))
		writePart(h, contents)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writePart(h hash.Hash, b []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(b)))
	_, _ = h.Write(length[:])
	_, _ = h.Write(b)
}
