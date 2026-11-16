package dist

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing/fstest"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

const (
	MaxArchiveBytes         = 10 * 1024 * 1024 // 10 MiB compressed cap
	MaxUncompressedTarBytes = 12 * 1024 * 1024 // 12 MiB tar stream cap (headers + 10 MiB payload)
)

type PackResult struct {
	KitID         string `json:"kitId"`
	Version       string `json:"version"`
	ArchivePath   string `json:"archivePath"`
	ChecksumPath  string `json:"checksumPath"`
	ArchiveSHA256 string `json:"archiveSha256"`
	PackageDigest string `json:"packageDigest"`
	SizeBytes     int64  `json:"sizeBytes"`
}

// Pack validates dir, renders all declared Codex scopes, and emits a deterministic .tar.gz archive + .sha256 file.
func Pack(dir, outPath string) (PackResult, error) {
	var res PackResult
	pkg, snap, pkgDigest, err := catalog.LoadDir(dir)
	if err != nil {
		return res, err
	}
	for _, sc := range pkg.Scopes {
		if _, err := codex.Render(pkg, snap, sc); err != nil {
			return res, err
		}
	}

	if outPath == "" {
		outPath = fmt.Sprintf("%s-%s.tar.gz", pkg.ID, pkg.Version)
	}
	absOut, err := filepath.Abs(outPath)
	if err != nil {
		return res, err
	}
	if info, statErr := os.Lstat(absOut); statErr == nil {
		if paths.IsReparseOrSymlink(info) || info.IsDir() {
			return res, apperr.WithPath(apperr.Usage("invalid_output_path", "output path cannot be a directory or symlink"), absOut)
		}
	} else if !os.IsNotExist(statErr) {
		return res, statErr
	}

	archiveBytes, archiveSHA256, err := BuildDeterministicArchive(snap)
	if err != nil {
		return res, err
	}
	if err := os.MkdirAll(filepath.Dir(absOut), 0755); err != nil {
		return res, err
	}
	if err := writeRegularFile(absOut, archiveBytes, 0644); err != nil {
		return res, err
	}
	sumPath := absOut + ".sha256"
	sumLine := fmt.Sprintf("%s  %s\n", archiveSHA256, filepath.Base(absOut))
	if err := writeRegularFile(sumPath, []byte(sumLine), 0644); err != nil {
		return res, err
	}

	return PackResult{
		KitID:         pkg.ID,
		Version:       pkg.Version,
		ArchivePath:   absOut,
		ChecksumPath:  sumPath,
		ArchiveSHA256: archiveSHA256,
		PackageDigest: pkgDigest,
		SizeBytes:     int64(len(archiveBytes)),
	}, nil
}

// BuildDeterministicArchive serializes a validated snapshot fs.FS into reproducible gzip+tar bytes.
func BuildDeterministicArchive(snap fs.FS) ([]byte, string, error) {
	var fileList []string
	err := fs.WalkDir(snap, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		fileList = append(fileList, name)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	sort.Strings(fileList)

	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, "", err
	}
	gz.ModTime = time.Unix(0, 0).UTC()
	gz.OS = 255
	tw := tar.NewWriter(gz)

	zeroTime := time.Unix(0, 0).UTC()
	for _, rel := range fileList {
		data, err := fs.ReadFile(snap, rel)
		if err != nil {
			return nil, "", err
		}
		hdr := &tar.Header{
			Name:     rel,
			Mode:     0644,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
			ModTime:  zeroTime,
			Uid:      0,
			Gid:      0,
			Uname:    "",
			Gname:    "",
			Format:   tar.FormatUSTAR,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, "", err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, "", err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, "", err
	}
	if err := gz.Close(); err != nil {
		return nil, "", err
	}
	out := buf.Bytes()
	if len(out) > MaxArchiveBytes {
		return nil, "", apperr.Usage("archive_too_large", "packed archive exceeds 10 MiB limit")
	}
	return out, sha256Hex(out), nil
}

// ReadArchiveFile loads a local .tar.gz archive safely into an immutable verified snapshot.
func ReadArchiveFile(archivePath, expectedArchiveSHA256 string) (catalog.Package, fs.FS, string, string, error) {
	abs, err := filepath.Abs(archivePath)
	if err != nil {
		return catalog.Package{}, nil, "", "", err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return catalog.Package{}, nil, "", "", apperr.WithPath(apperr.Usage("archive_not_found", "package archive does not exist"), abs)
		}
		return catalog.Package{}, nil, "", "", err
	}
	if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
		return catalog.Package{}, nil, "", "", apperr.WithPath(apperr.Usage("invalid_archive", "archive must be a regular file (not symlink or directory)"), abs)
	}
	if info.Size() > MaxArchiveBytes {
		return catalog.Package{}, nil, "", "", apperr.WithPath(apperr.Usage("archive_too_large", "package archive exceeds 10 MiB limit"), abs)
	}
	f, err := os.Open(abs)
	if err != nil {
		return catalog.Package{}, nil, "", "", err
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, MaxArchiveBytes+1))
	closeErr := f.Close()
	if readErr != nil {
		return catalog.Package{}, nil, "", "", readErr
	}
	if closeErr != nil {
		return catalog.Package{}, nil, "", "", closeErr
	}
	return ReadArchiveBytes(raw, expectedArchiveSHA256)
}

// ReadArchiveBytes validates and extracts a .tar.gz archive purely in memory, rejecting
// symlinks, hardlinks, device nodes, traversal, case collisions, zip-bombs, and invalid manifests.
func ReadArchiveBytes(raw []byte, expectedArchiveSHA256 string) (catalog.Package, fs.FS, string, string, error) {
	if len(raw) > MaxArchiveBytes {
		return catalog.Package{}, nil, "", "", apperr.Usage("archive_too_large", "archive exceeds 10 MiB compressed limit")
	}
	actualArchiveSHA256 := sha256Hex(raw)
	if expectedArchiveSHA256 != "" && !strings.EqualFold(actualArchiveSHA256, expectedArchiveSHA256) {
		return catalog.Package{}, nil, "", "", apperr.Conflict("archive_digest_mismatch", fmt.Sprintf("archive SHA-256 %s does not match expected %s", actualArchiveSHA256, expectedArchiveSHA256))
	}

	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return catalog.Package{}, nil, "", "", apperr.Wrap(apperr.ExitUsage, "invalid_archive_gzip", fmt.Sprintf("invalid gzip archive: %v", err), err)
	}
	defer gz.Close()

	limitedStream := io.LimitReader(gz, MaxUncompressedTarBytes+1)
	tr := tar.NewReader(limitedStream)

	memFS := fstest.MapFS{}
	seenPaths := map[string]string{}
	fileCount := 0
	var totalBytes int64

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return catalog.Package{}, nil, "", "", apperr.Wrap(apperr.ExitUsage, "invalid_archive_tar", fmt.Sprintf("corrupt or truncated tar stream: %v", err), err)
		}

		cleanName, err := sanitizeArchiveEntryName(hdr.Name, hdr.Typeflag == tar.TypeDir)
		if err != nil {
			return catalog.Package{}, nil, "", "", err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if cleanName == "." {
				continue
			}
			key := strings.ToLower(cleanName)
			if prev, ok := seenPaths[key]; ok {
				return catalog.Package{}, nil, "", "", apperr.WithPath(apperr.Usage("archive_path_collision", fmt.Sprintf("case-insensitive collision between %q and %q", prev, cleanName)), cleanName)
			}
			seenPaths[key] = cleanName
			continue
		case tar.TypeReg:
			if cleanName == "." {
				return catalog.Package{}, nil, "", "", apperr.Usage("invalid_archive_path", "regular file cannot be named '.'")
			}
		default:
			return catalog.Package{}, nil, "", "", apperr.WithPath(apperr.Usage("unsafe_archive_entry", fmt.Sprintf("archive entry %q has unsupported type %d (links/devices prohibited)", hdr.Name, hdr.Typeflag)), hdr.Name)
		}

		if hdr.Size < 0 || hdr.Size > catalog.MaxFileBytes {
			return catalog.Package{}, nil, "", "", apperr.WithPath(apperr.Usage("archive_file_too_large", fmt.Sprintf("archive entry %q exceeds 1 MiB file limit", cleanName)), cleanName)
		}
		key := strings.ToLower(cleanName)
		if prev, ok := seenPaths[key]; ok {
			return catalog.Package{}, nil, "", "", apperr.WithPath(apperr.Usage("archive_path_collision", fmt.Sprintf("duplicate or case-colliding archive entry %q and %q", prev, cleanName)), cleanName)
		}
		seenPaths[key] = cleanName

		fileCount++
		if fileCount > catalog.MaxFiles {
			return catalog.Package{}, nil, "", "", apperr.Usage("archive_file_count_limit", "archive exceeds 512 file limit")
		}

		content, err := io.ReadAll(io.LimitReader(tr, catalog.MaxFileBytes+1))
		if err != nil {
			return catalog.Package{}, nil, "", "", apperr.Wrap(apperr.ExitUsage, "invalid_archive_tar", fmt.Sprintf("read %q: %v", cleanName, err), err)
		}
		if len(content) > catalog.MaxFileBytes {
			return catalog.Package{}, nil, "", "", apperr.WithPath(apperr.Usage("archive_file_too_large", fmt.Sprintf("archive entry %q exceeds 1 MiB limit", cleanName)), cleanName)
		}
		totalBytes += int64(len(content))
		if totalBytes > catalog.MaxPackageBytes {
			return catalog.Package{}, nil, "", "", apperr.Usage("archive_size_limit", "extracted package exceeds 10 MiB limit")
		}
		memFS[cleanName] = &fstest.MapFile{Data: content, Mode: 0644}
	}

	pkg, err := catalog.Load(memFS)
	if err != nil {
		return catalog.Package{}, nil, "", "", err
	}
	snap, pkgDigest, err := catalog.Snapshot(memFS)
	if err != nil {
		return catalog.Package{}, nil, "", "", err
	}
	for _, sc := range pkg.Scopes {
		if _, err := codex.Render(pkg, snap, sc); err != nil {
			return catalog.Package{}, nil, "", "", err
		}
	}
	return pkg, snap, pkgDigest, actualArchiveSHA256, nil
}

func sanitizeArchiveEntryName(raw string, isDir bool) (string, error) {
	if strings.ContainsAny(raw, "\\:") || strings.HasPrefix(raw, "/") {
		return "", apperr.WithPath(apperr.Usage("unsafe_archive_path", fmt.Sprintf("unsafe archive entry path %q", raw)), raw)
	}
	trimmed := strings.TrimPrefix(raw, "./")
	if isDir {
		trimmed = strings.TrimSuffix(trimmed, "/")
	}
	if trimmed == "" || trimmed == "." {
		if isDir {
			return ".", nil
		}
		return "", apperr.WithPath(apperr.Usage("unsafe_archive_path", fmt.Sprintf("invalid archive file path %q", raw)), raw)
	}
	cleaned := path.Clean(trimmed)
	if cleaned != trimmed || !paths.ValidateRelativePath(cleaned) {
		return "", apperr.WithPath(apperr.Usage("unsafe_archive_path", fmt.Sprintf("unsafe archive path %q", raw)), raw)
	}
	return cleaned, nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
