package dist

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestLockfileSchemaPortabilityAndDiff(t *testing.T) {
	valid := `{
  "schemaVersion": 1,
  "kits": [
    {
      "id": "engineer",
      "version": "0.4.0",
      "packageDigest": "` + strings.Repeat("a", 64) + `",
      "source": { "kind": "bundled" },
      "runtimeTargets": ["codex", "claude"],
      "adapterRevisions": { "codex": "1", "claude": "1" },
      "selection": { "preset": "minimal", "resolvedComponents": ["rule/engineering", "skill/ask"] }
    }
  ]
}`
	lf1, err := ParseAndValidateLockfile([]byte(valid))
	if err != nil {
		t.Fatalf("valid lockfile rejected: %v", err)
	}
	marshaled, err := MarshalLockfile(lf1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseAndValidateLockfile(marshaled); err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}

	// Reject non-portable refs, embedded tokens, unknown fields, and duplicate kits.
	badCases := []struct {
		name string
		raw  string
	}{
		{
			name: "absolute_unix_path",
			raw:  strings.Replace(valid, `{ "kind": "bundled" }`, `{ "kind": "local", "ref": "/home/alice/kits/engineer" }`, 1),
		},
		{
			name: "windows_drive_path",
			raw:  strings.Replace(valid, `{ "kind": "bundled" }`, `{ "kind": "local", "ref": "C:\\Users\\alice\\kits\\engineer" }`, 1),
		},
		{
			name: "home_tilde_path",
			raw:  strings.Replace(valid, `{ "kind": "bundled" }`, `{ "kind": "local", "ref": "~/kits/engineer" }`, 1),
		},
		{
			name: "parent_traversal",
			raw:  strings.Replace(valid, `{ "kind": "bundled" }`, `{ "kind": "local", "ref": "../outside/engineer" }`, 1),
		},
		{
			name: "remote_query_token",
			raw:  `{"schemaVersion":1,"kits":[{"id":"acme","version":"0.1.0","packageDigest":"` + strings.Repeat("a", 64) + `","archiveSha256":"` + strings.Repeat("b", 64) + `","source":{"kind":"remote","ref":"https://example.com/idx.json?token=secret"},"runtimeTargets":["codex"]}]}`,
		},
		{
			name: "unknown_top_field",
			raw:  `{"schemaVersion":1,"extra":true,"kits":[]}`,
		},
	}
	for _, tc := range badCases {
		if _, err := ParseAndValidateLockfile([]byte(tc.raw)); err == nil {
			t.Fatalf("%s: expected validation error", tc.name)
		}
	}

	lf2 := Lockfile{
		SchemaVersion: 1,
		Kits: []LockedKit{
			{
				ID:             "engineer",
				Version:        "0.4.1",
				PackageDigest:  strings.Repeat("c", 64),
				Source:         LockSource{Kind: "local", Ref: "kits/engineer"},
				RuntimeTargets: []string{"codex", "claude"},
				Selection:      &catalog.SelectionSpec{Preset: "engineering", ResolvedComponents: []string{"rule/engineering", "skill/ask", "skill/review"}},
			},
			{
				ID:             "security-review",
				Version:        "0.1.0",
				PackageDigest:  strings.Repeat("d", 64),
				Source:         LockSource{Kind: "local", Ref: "kits/security"},
				RuntimeTargets: []string{"claude"},
			},
		},
	}
	diff := DiffLockfiles(lf1, lf2)
	if len(diff.Changes) != 2 {
		t.Fatalf("expected 2 diff changes, got %+v", diff)
	}
	if diff.Changes[0].KitID != "engineer" || !diff.Changes[0].SourceChanged || !diff.Changes[0].VersionChanged || !diff.Changes[0].DigestChanged || !diff.Changes[0].SelectionChanged {
		t.Fatalf("unexpected engineer diff flags: %+v", diff.Changes[0])
	}
	if diff.Changes[1].KitID != "security-review" || diff.Changes[1].Status != "added" {
		t.Fatalf("unexpected added diff entry: %+v", diff.Changes[1])
	}
}

func TestProvenanceSigningRotationRevocationAndExpiration(t *testing.T) {
	pub1, priv1, err := GenerateEd25519Keypair()
	if err != nil {
		t.Fatal(err)
	}
	pub2, priv2, err := GenerateEd25519Keypair()
	if err != nil {
		t.Fatal(err)
	}

	dig := strings.Repeat("a", 64)
	prov1, err := SignDigest("acme", "0.1.0", dig, "k1", priv1)
	if err != nil {
		t.Fatal(err)
	}
	prov2, err := SignDigest("acme", "0.1.0", dig, "k2", priv2)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 11, 17, 14, 0, 0, 0, time.UTC)

	// Rotation: k1 and k2 both active -> both verify.
	policy := TrustPolicy{
		SchemaVersion:    1,
		RequireSignature: true,
		Keys: []TrustedKey{
			{KeyID: "k1", Publisher: "acme-ci", Algorithm: "ed25519", PublicKey: pub1, Status: "active", NotAfter: "2027-01-01T00:00:00Z"},
			{KeyID: "k2", Publisher: "acme-ci", Algorithm: "ed25519", PublicKey: pub2, Status: "active", AllowedKits: []string{"acme"}},
		},
	}
	polBytes, err := MarshalTrustPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	parsedPol, err := ParseAndValidateTrustPolicy(polBytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyProvenance("acme", "0.1.0", dig, &prov1, &parsedPol, false, now); err != nil {
		t.Fatalf("k1 should verify while active: %v", err)
	}
	if err := VerifyProvenance("acme", "0.1.0", dig, &prov2, &parsedPol, false, now); err != nil {
		t.Fatalf("k2 should verify while active: %v", err)
	}

	// Missing signature rejected when requireSignature=true.
	var appErr *apperr.Error
	if err := VerifyProvenance("acme", "0.1.0", dig, nil, &parsedPol, false, now); !errors.As(err, &appErr) || appErr.Code != "signature_required" {
		t.Fatalf("expected signature_required, got %v", err)
	}

	// Disallowed kit for k2 fails closed.
	provOther, err := SignDigest("other-kit", "0.1.0", dig, "k2", priv2)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyProvenance("other-kit", "0.1.0", dig, &provOther, &parsedPol, false, now); !errors.As(err, &appErr) || appErr.Code != "key_kit_disallowed" {
		t.Fatalf("expected key_kit_disallowed, got %v", err)
	}

	// Tampered digest or signature fails closed.
	if err := VerifyProvenance("acme", "0.1.0", strings.Repeat("b", 64), &prov1, &parsedPol, false, now); !errors.As(err, &appErr) || appErr.Code != "provenance_digest_mismatch" {
		t.Fatalf("expected provenance_digest_mismatch, got %v", err)
	}

	// Revoking k1 immediately rejects k1 while k2 remains valid.
	parsedPol.Keys[0].Status = "revoked"
	if err := VerifyProvenance("acme", "0.1.0", dig, &prov1, &parsedPol, false, now); !errors.As(err, &appErr) || appErr.Code != "key_revoked" {
		t.Fatalf("expected key_revoked after revocation, got %v", err)
	}
	if err := VerifyProvenance("acme", "0.1.0", dig, &prov2, &parsedPol, false, now); err != nil {
		t.Fatalf("k2 should still verify after k1 revoked: %v", err)
	}

	// Expired key fails closed.
	parsedPol.Keys[0].Status = "active"
	parsedPol.Keys[0].NotAfter = "2026-01-01T00:00:00Z"
	if err := VerifyProvenance("acme", "0.1.0", dig, &prov1, &parsedPol, false, now); !errors.As(err, &appErr) || appErr.Code != "key_expired" {
		t.Fatalf("expected key_expired, got %v", err)
	}
}

func TestResolveLockedKitOfflineCacheAndUnavailable(t *testing.T) {
	cacheDir := t.TempDir()
	cache, err := OpenCache(paths.Environment{UserCacheDir: func() (string, error) { return cacheDir, nil }})
	if err != nil {
		t.Fatal(err)
	}

	// Build a deterministic custom package archive in memory.
	pkgJSON := `{
  "schemaVersion": 1,
  "id": "acme",
  "name": "Acme Kit",
  "version": "0.1.0",
  "description": "Custom locked kit.",
  "runtimeTargets": ["codex"],
  "scopes": ["project"],
  "requiresCapabilities": ["skills"],
  "packageDependencies": [],
  "components": [
    { "id": "rule/base", "kind": "rule", "source": "rules/base.md", "requires": [] },
    { "id": "skill/audit", "kind": "skill", "source": "skills/audit", "requires": ["rule/base"] }
  ]
}`
	fsMap := fstest.MapFS{
		"package.json":          &fstest.MapFile{Data: []byte(pkgJSON)},
		"rules/base.md":         &fstest.MapFile{Data: []byte("# Base\n")},
		"skills/audit/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: dtkit-project-acme-audit\ndescription: Audit carefully.\n---\n\n# Audit\n\n## When to use\n- Audit code.\n\n## When not to use\n- Do not edit.\n\n## Steps\n1. Inspect.\n\n## Constraints\n- Be accurate.\n\n## Examples\n1. Check.\n2. Report.\n3. Ask.\n")},
	}
	pkgDig, err := catalog.Digest(fsMap)
	if err != nil {
		t.Fatal(err)
	}
	archBytes, archSHA, err := BuildDeterministicArchive(fsMap)
	if err != nil {
		t.Fatal(err)
	}

	entry := LockedKit{
		ID:             "acme",
		Version:        "0.1.0",
		PackageDigest:  pkgDig,
		ArchiveSHA256:  archSHA,
		Source:         LockSource{Kind: "remote", Ref: "https://registry.example.com/index.json"},
		RuntimeTargets: []string{"codex"},
	}

	// 1. Offline with empty cache -> offline_cache_miss.
	var appErr *apperr.Error
	_, err = ResolveLockedKit(context.Background(), nil, cache, t.TempDir(), entry, "0.5.0-dev", true, CacheReadOnly)
	if !errors.As(err, &appErr) || appErr.Code != "offline_cache_miss" {
		t.Fatalf("expected offline_cache_miss, got %v", err)
	}

	// 2. Populate cache -> offline resolution hits exact pinned bytes cleanly.
	sum := sha256.Sum256(archBytes)
	if hex.EncodeToString(sum[:]) != archSHA {
		t.Fatal("sha mismatch")
	}
	if _, _, err := cache.PutArchive(archBytes, &CacheMetadata{SchemaVersion: 1, ArchiveSHA256: archSHA, PackageDigest: pkgDig}); err != nil {
		t.Fatal(err)
	}
	res, err := ResolveLockedKit(context.Background(), nil, cache, t.TempDir(), entry, "0.5.0-dev", true, CacheReadOnly)
	if err != nil || res.Package.ID != "acme" || res.ArchiveSHA256 != archSHA || res.PackageDigest != pkgDig {
		t.Fatalf("expected offline cache hit resolution, got res=%+v err=%v", res, err)
	}

	// 3. Tampered lockfile packageDigest fails closed.
	badEntry := entry
	badEntry.PackageDigest = strings.Repeat("f", 64)
	if _, err := ResolveLockedKit(context.Background(), nil, cache, t.TempDir(), badEntry, "0.5.0-dev", true, CacheReadOnly); !errors.As(err, &appErr) || appErr.Code != "lock_digest_mismatch" {
		t.Fatalf("expected lock_digest_mismatch, got %v", err)
	}
}
