package catalog

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"io"
	"io/fs"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
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
		if entry.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			return apperr.WithPath(apperr.Usage("symlink_not_allowed", "symbolic link not allowed"), name)
		}
		file, err := source.Open(name)
		if err != nil {
			return err
		}
		contents, readErr := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		count++
		total += int64(len(contents))
		if len(contents) > MaxFileBytes || count > MaxFiles || total > MaxPackageBytes {
			return apperr.WithPath(apperr.Usage("package_size_limit", "package size limit exceeded"), name)
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
