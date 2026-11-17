package dist

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

// ForkOptions configures controlled export/forking of a verified source kit into a local directory.
type ForkOptions struct {
	TargetID    string
	DisplayName string
	Version     string
	AllowSameID bool
}

// ForkResult summarizes a forked local kit directory written by ForkKit.
type ForkResult struct {
	SourceID      string `json:"sourceId"`
	SourceVersion string `json:"sourceVersion"`
	ID            string `json:"id"`
	Version       string `json:"version"`
	Dir           string `json:"dir"`
	Digest        string `json:"digest"`
	FileCount     int    `json:"fileCount"`
}

// ForkKit exports a verified source kit into a standalone local package directory at
// <parentDir>/<targetID> so users can customize and version-bump without editing managed runtime files.
func ForkKit(parentDir string, srcPkg catalog.Package, srcFS fs.FS, opts ForkOptions) (ForkResult, error) {
	var res ForkResult
	targetID := strings.TrimSpace(opts.TargetID)
	if targetID == "" {
		targetID = srcPkg.ID + "-custom"
	}
	if !catalog.IsValidID(targetID, 32) {
		return res, apperr.Usage("invalid_kit_id", fmt.Sprintf("kit ID %q must be <=32 lowercase ASCII letters/digits/hyphens", targetID))
	}
	if targetID == srcPkg.ID && !opts.AllowSameID {
		return res, apperr.WithHint(
			apperr.Conflict("duplicate_kit_id", fmt.Sprintf("fork target ID %q duplicates source kit ID", targetID)),
			fmt.Sprintf("choose a distinct --as <id> (for example %s-custom) or pass --same-id to create a higher-version local override", srcPkg.ID),
		)
	}

	nextVersion := strings.TrimSpace(opts.Version)
	if nextVersion != "" {
		if !catalog.IsValidVersion(nextVersion) {
			return res, apperr.Usage("invalid_version", fmt.Sprintf("version %q must be strict X.Y.Z semver", nextVersion))
		}
		if opts.AllowSameID && targetID == srcPkg.ID {
			cmp, err := compareStrictSemver(nextVersion, srcPkg.Version)
			if err != nil {
				return res, err
			}
			if cmp <= 0 {
				return res, apperr.WithHint(
					apperr.Conflict("version_not_bumped", fmt.Sprintf("fork version %s must be higher than source version %s when using --same-id", nextVersion, srcPkg.Version)),
					fmt.Sprintf("pass --version %s or omit --version to auto-bump patch", bumpPatchSemver(srcPkg.Version)),
				)
			}
		}
	} else if targetID == srcPkg.ID {
		nextVersion = bumpPatchSemver(srcPkg.Version)
	} else {
		nextVersion = "0.1.0"
	}

	absParent, err := filepath.Abs(parentDir)
	if err != nil {
		return res, err
	}
	if info, err := os.Lstat(absParent); err == nil {
		if paths.IsReparseOrSymlink(info) {
			return res, apperr.WithPath(apperr.Usage("symlink_not_allowed", "parent directory cannot be a symlink or reparse point"), absParent)
		}
		if !info.IsDir() {
			return res, apperr.WithPath(apperr.Usage("invalid_parent_dir", "parent path is not a directory"), absParent)
		}
	} else if !os.IsNotExist(err) {
		return res, err
	}

	kitDir := filepath.Join(absParent, targetID)
	if _, err := os.Lstat(kitDir); err == nil {
		return res, apperr.WithPath(apperr.Conflict("target_exists", "fork target directory already exists"), kitDir)
	} else if !os.IsNotExist(err) {
		return res, err
	}

	if err := os.MkdirAll(kitDir, 0755); err != nil {
		return res, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(kitDir)
		}
	}()

	fileCount := 0
	var totalBytes int64
	if err := fs.WalkDir(srcFS, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if rel == "." || rel == "package.json" {
			return nil
		}
		if !paths.ValidateRelativePath(rel) {
			return apperr.Usage("invalid_package_path", fmt.Sprintf("invalid package path %q", rel))
		}
		if d.Type()&os.ModeSymlink != 0 {
			return apperr.Usage("symlink_not_allowed", fmt.Sprintf("source package contains symlink %q", rel))
		}
		dstPath, err := paths.ResolveTarget(kitDir, rel)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}
		if !d.Type().IsRegular() {
			return apperr.Usage("invalid_package_entry", fmt.Sprintf("non-regular file %q in source package", rel))
		}
		data, err := fs.ReadFile(srcFS, rel)
		if err != nil {
			return err
		}
		if int64(len(data)) > catalog.MaxFileBytes {
			return apperr.Usage("file_too_large", fmt.Sprintf("file %q exceeds %d byte limit", rel, catalog.MaxFileBytes))
		}
		totalBytes += int64(len(data))
		if totalBytes > catalog.MaxPackageBytes {
			return apperr.Usage("package_too_large", "forked package exceeds size limit")
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}
		if err := writeRegularFile(dstPath, data, 0644); err != nil {
			return err
		}
		fileCount++
		return nil
	}); err != nil {
		return res, err
	}

	// Copy metadata cleanly without embedding source filesystem paths, home dirs, or URLs.
	forkedPkg := clonePackageMetadata(srcPkg)
	forkedPkg.ID = targetID
	forkedPkg.Version = nextVersion
	if strings.TrimSpace(opts.DisplayName) != "" {
		forkedPkg.Name = strings.TrimSpace(opts.DisplayName)
	} else if targetID != srcPkg.ID {
		forkedPkg.Name = srcPkg.Name + " (" + targetID + ")"
	}

	manifestBytes, err := json.MarshalIndent(forkedPkg, "", "  ")
	if err != nil {
		return res, err
	}
	if err := writeRegularFile(filepath.Join(kitDir, "package.json"), append(manifestBytes, '\n'), 0644); err != nil {
		return res, err
	}
	fileCount++

	loaded, snap, digest, err := catalog.LoadDir(kitDir)
	if err != nil {
		return res, err
	}
	if err := validatePackageRenderTargets(loaded, snap); err != nil {
		return res, err
	}
	committed = true
	return ForkResult{
		SourceID:      srcPkg.ID,
		SourceVersion: srcPkg.Version,
		ID:            loaded.ID,
		Version:       loaded.Version,
		Dir:           kitDir,
		Digest:        digest,
		FileCount:     fileCount,
	}, nil
}

func cloneStringSliceNonNil(src []string) []string {
	out := make([]string, len(src))
	copy(out, src)
	return out
}

func clonePackageMetadata(src catalog.Package) catalog.Package {
	out := src
	out.RuntimeTargets = cloneStringSliceNonNil(src.RuntimeTargets)
	out.Scopes = cloneStringSliceNonNil(src.Scopes)
	out.RequiresCapabilities = cloneStringSliceNonNil(src.RequiresCapabilities)
	if src.OptionalCapabilities != nil {
		out.OptionalCapabilities = cloneStringSliceNonNil(src.OptionalCapabilities)
	}
	out.PackageDependencies = cloneStringSliceNonNil(src.PackageDependencies)
	out.Components = make([]catalog.Component, len(src.Components))
	for i, c := range src.Components {
		cCopy := c
		cCopy.Requires = cloneStringSliceNonNil(c.Requires)
		out.Components[i] = cCopy
	}
	if len(src.Presets) > 0 {
		out.Presets = make(map[string][]string, len(src.Presets))
		for k, v := range src.Presets {
			out.Presets[k] = cloneStringSliceNonNil(v)
		}
	}
	return out
}

func bumpPatchSemver(v string) string {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return "0.1.0"
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil || patch < 0 {
		return "0.1.0"
	}
	return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], patch+1)
}

func compareStrictSemver(a, b string) (int, error) {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	if len(pa) != 3 || len(pb) != 3 {
		return 0, apperr.Usage("invalid_version", fmt.Sprintf("invalid semver comparison %q vs %q", a, b))
	}
	for i := 0; i < 3; i++ {
		na, errA := strconv.Atoi(pa[i])
		nb, errB := strconv.Atoi(pb[i])
		if errA != nil || errB != nil {
			return 0, apperr.Usage("invalid_version", fmt.Sprintf("invalid semver comparison %q vs %q", a, b))
		}
		if na < nb {
			return -1, nil
		}
		if na > nb {
			return 1, nil
		}
	}
	return 0, nil
}
