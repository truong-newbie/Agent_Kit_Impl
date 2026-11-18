package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/buildinfo"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

func runLock(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("lock")
	jsonOutput := hasJSONFlag(args)
	redact := hasFlag(args, "--redact")
	fail := func(err error) error {
		if redact {
			err = redactAppError(err, collectHomeCandidates(deps))
		}
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	if len(args) == 0 {
		return fail(apperr.Usage("missing_subcommand", "usage: dtkit lock <generate|verify|diff|sign|keygen> [options]"))
	}
	sub := args[0]
	env.Command = "lock " + sub

	switch sub {
	case "generate":
		return runLockGenerate(ctx, args[1:], deps, env, fail)
	case "verify":
		return runLockVerify(ctx, args[1:], deps, env, fail)
	case "diff":
		return runLockDiff(ctx, args[1:], deps, env, fail)
	case "sign":
		return runLockSign(ctx, args[1:], deps, env, fail)
	case "keygen":
		return runLockKeygen(args[1:], deps, env, fail)
	default:
		return fail(apperr.Usage("unknown_subcommand", fmt.Sprintf("unknown lock subcommand %q (expected generate, verify, diff, sign, keygen)", sub)))
	}
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func runLockGenerate(ctx context.Context, args []string, deps Deps, env Envelope, fail func(error) error) error {
	var projectPath, runtimeArg, outPath, signKeyFile, keyID string
	var dryRun, yes, jsonOutput, redact bool
	sourceRefs := map[string]string{}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run":
			dryRun = true
		case "--yes":
			yes = true
		case "--json":
			jsonOutput = true
		case "--redact":
			redact = true
		case "--verbose":
			deps.Verbose = true
		case "--path", "--runtime", "--out", "--sign-key", "--key-id", "--source-ref":
			flag := args[i]
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
			}
			switch flag {
			case "--path":
				projectPath = args[i]
			case "--runtime":
				runtimeArg = args[i]
			case "--out":
				outPath = args[i]
			case "--sign-key":
				signKeyFile = args[i]
			case "--key-id":
				keyID = args[i]
			case "--source-ref":
				kitID, ref, ok := strings.Cut(args[i], "=")
				if !ok || strings.TrimSpace(kitID) == "" || strings.TrimSpace(ref) == "" {
					return fail(apperr.Usage("invalid_source_ref", "--source-ref must use <kit-id>=<relative-path-or-url>"))
				}
				sourceRefs[strings.TrimSpace(kitID)] = strings.TrimSpace(ref)
			}
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown lock generate option %q", args[i])))
		}
	}
	if (signKeyFile != "") != (keyID != "") {
		return fail(apperr.Usage("invalid_flag_combination", "--sign-key and --key-id must be passed together"))
	}

	profile, err := resolveRuntimeProfile(runtimeArg)
	if err != nil {
		return fail(err)
	}
	env.DryRun = dryRun
	env.Runtime = profile.ID
	env.Scope = "project"

	roots, rootWarnings, err := resolveScopeRoots(ctx, deps, profile, "project", projectPath)
	if err != nil {
		return fail(err)
	}
	env.Warnings = append(env.Warnings, rootWarnings...)
	env.Roots = roots
	projRoot := roots["project"]

	statePath, err := resolveRuntimeStatePath(profile, "project", roots)
	if err != nil {
		return fail(err)
	}
	reg, exists, err := state.LoadRegistryForRuntime(statePath, "project", profile.ID)
	if err != nil {
		return fail(err)
	}
	if !exists || len(reg.Kits) == 0 {
		return fail(apperr.Conflict("not_installed", fmt.Sprintf("no kits installed in project scope (%s) to lock", profile.ID)))
	}

	var privKeyB64 string
	if signKeyFile != "" {
		rawKey, rErr := os.ReadFile(signKeyFile)
		if rErr != nil {
			return fail(apperr.WithPath(apperr.Usage("key_read_failed", "unable to read signing private key"), signKeyFile))
		}
		privKeyB64 = strings.TrimSpace(string(rawKey))
	}

	nextLF := dist.Lockfile{
		SchemaVersion: 1,
		Kits:          make([]dist.LockedKit, 0, len(reg.KitIDs())),
	}
	for _, kitID := range reg.KitIDs() {
		inst := reg.Kits[kitID]
		src, archSHA, pkgTargets, err := resolveLockSourceForInstalled(projRoot, inst, sourceRefs[kitID])
		if err != nil {
			return fail(err)
		}
		if len(pkgTargets) == 0 {
			pkgTargets = []string{profile.ID}
		}
		entry := dist.LockedKit{
			ID:               inst.Kit.ID,
			Version:          inst.Kit.Version,
			PackageDigest:    inst.Kit.Digest,
			ArchiveSHA256:    archSHA,
			Source:           src,
			RuntimeTargets:   pkgTargets,
			AdapterRevisions: map[string]string{profile.ID: profile.Revision},
			Selection:        inst.Selection,
		}
		if privKeyB64 != "" {
			prov, sErr := dist.SignDigest(entry.ID, entry.Version, entry.LockTargetDigest(), keyID, privKeyB64)
			if sErr != nil {
				return fail(sErr)
			}
			entry.Provenance = &prov
		}
		nextLF.Kits = append(nextLF.Kits, entry)
	}

	lockTargetPath := filepath.Join(projRoot, dist.LockfileRelPath)
	if outPath != "" {
		lockTargetPath, err = filepath.Abs(outPath)
		if err != nil {
			return fail(err)
		}
	}

	serialized, err := dist.MarshalLockfile(nextLF)
	if err != nil {
		return fail(err)
	}
	validatedLF, err := dist.ParseAndValidateLockfile(serialized)
	if err != nil {
		return fail(err)
	}
	env.Lockfile = &validatedLF

	if oldRaw, rErr := readRegularFileBounded(lockTargetPath, dist.MaxLockfileBytes); rErr == nil {
		if oldLF, pErr := dist.ParseAndValidateLockfile(oldRaw); pErr == nil {
			diff := dist.DiffLockfiles(oldLF, validatedLF)
			env.LockDiff = &diff
		}
	}

	if dryRun {
		if redact {
			env.Roots = cloneRootsMap(roots)
			redactEnvelope(&env, collectHomeCandidates(deps))
		}
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Read-only preview: dtkit.lock would pin %d kit(s) at %s\n", len(validatedLF.Kits), lockTargetPath)
		printLockDiffText(deps.Stdout, env.LockDiff)
		return nil
	}

	if jsonOutput && !yes {
		return fail(apperr.WithHint(apperr.Usage("confirmation_required", "--json mutation requires --yes"), "pass --dry-run to preview or --yes to write dtkit.lock"))
	}
	if !yes && !deps.IsInteractive() {
		return fail(apperr.WithHint(apperr.Usage("confirmation_required", "non-interactive lock generate requires --yes; run --dry-run first"), "run with --dry-run then --yes"))
	}
	if !yes {
		confirmed, err := confirmPrompt(ctx, deps, fmt.Sprintf("Write %s?", lockTargetPath))
		if err != nil {
			return fail(err)
		}
		if !confirmed {
			return fail(apperr.Cancelled("lock generation cancelled by user"))
		}
	}

	if err := atomicWriteFile(lockTargetPath, serialized, 0644); err != nil {
		return fail(err)
	}

	if redact {
		env.Roots = cloneRootsMap(roots)
		redactEnvelope(&env, collectHomeCandidates(deps))
	}
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintf(deps.Stdout, "Wrote %s (%d locked kit(s)).\n", lockTargetPath, len(validatedLF.Kits))
	return nil
}

func resolveLockSourceForInstalled(projectRoot string, inst state.Installation, refOverride string) (dist.LockSource, string, []string, error) {
	archSHA := inst.Kit.ArchiveSHA256
	if refOverride != "" {
		if strings.HasPrefix(refOverride, "https://") || strings.HasPrefix(refOverride, "http://") {
			if err := dist.ValidateRemoteURL(refOverride); err != nil {
				return dist.LockSource{}, "", nil, err
			}
			if archSHA == "" {
				return dist.LockSource{}, "", nil, apperr.Usage("missing_archive_sha256", fmt.Sprintf("remote source for %s requires installed archiveSha256", inst.Kit.ID))
			}
			return dist.LockSource{Kind: "remote", Ref: dist.SanitizeURL(refOverride)}, archSHA, nil, nil
		}
		rel, err := relativizeWithinProject(projectRoot, refOverride)
		if err != nil {
			return dist.LockSource{}, "", nil, err
		}
		absRef, err := paths.ResolveTarget(projectRoot, rel)
		if err != nil {
			return dist.LockSource{}, "", nil, err
		}
		resolved, err := dist.ResolveFromPath(absRef, inst.Kit.ID, inst.Kit.Version, archSHA)
		if err != nil {
			return dist.LockSource{}, "", nil, err
		}
		if resolved.PackageDigest != inst.Kit.Digest {
			return dist.LockSource{}, "", nil, apperr.Conflict("lock_digest_mismatch", fmt.Sprintf("--source-ref %s digest %s does not match installed %s digest %s", rel, resolved.PackageDigest, inst.Kit.ID, inst.Kit.Digest))
		}
		kind := "local"
		if resolved.SourceKind == "archive" {
			kind = "archive"
			archSHA = resolved.ArchiveSHA256
		}
		return dist.LockSource{Kind: kind, Ref: rel}, archSHA, resolved.Package.RuntimeTargets, nil
	}

	if inst.Kit.ID == "engineer" && (inst.Kit.SourceKind == "" || inst.Kit.SourceKind == "bundled") {
		pkg, srcFS, err := catalog.Engineer()
		if err == nil && pkg.Version == inst.Kit.Version {
			if overFS, _, oErr := catalog.ApplyProjectOverrides(projectRoot, pkg, srcFS); oErr == nil {
				srcFS = overFS
			}
			if dig, dErr := catalog.Digest(srcFS); dErr == nil && dig == inst.Kit.Digest {
				return dist.LockSource{Kind: "bundled"}, "", pkg.RuntimeTargets, nil
			}
		}
	}

	return dist.LockSource{}, "", nil, apperr.WithHint(
		apperr.Usage("missing_source_ref", fmt.Sprintf("cannot infer portable lock source for %s %s (%s)", inst.Kit.ID, inst.Kit.Version, inst.Kit.SourceKind)),
		fmt.Sprintf("pass --source-ref %s=<relative-path-or-https-url>", inst.Kit.ID),
	)
}

func relativizeWithinProject(projectRoot, ref string) (string, error) {
	cleaned := filepath.ToSlash(strings.TrimSpace(ref))
	if filepath.IsAbs(ref) {
		rel, err := filepath.Rel(projectRoot, ref)
		if err != nil {
			return "", apperr.Usage("non_portable_lock_ref", fmt.Sprintf("source ref %q must reside within project root", ref))
		}
		cleaned = filepath.ToSlash(rel)
	}
	if !paths.ValidateRelativePath(cleaned) {
		return "", apperr.Usage("non_portable_lock_ref", fmt.Sprintf("source ref %q must be a safe project-relative path", ref))
	}
	return cleaned, nil
}

func runLockVerify(ctx context.Context, args []string, deps Deps, env Envelope, fail func(error) error) error {
	var projectPath, lockPath, trustPath string
	var offline, requireSig, jsonOutput, redact bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--offline":
			offline = true
		case "--require-signature":
			requireSig = true
		case "--json":
			jsonOutput = true
		case "--redact":
			redact = true
		case "--verbose":
			deps.Verbose = true
		case "--path", "--lock", "--trust":
			flag := args[i]
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
			}
			switch flag {
			case "--path":
				projectPath = args[i]
			case "--lock":
				lockPath = args[i]
			case "--trust":
				trustPath = args[i]
			}
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown lock verify option %q", args[i])))
		}
	}

	projRes, err := paths.ResolveProject(ctx, deps.Env, projectPath)
	if err != nil {
		return fail(err)
	}
	env.Warnings = append(env.Warnings, projRes.Warnings...)
	env.Roots = map[string]string{"project": projRes.Root}
	env.DryRun = true
	env.Scope = "project"

	lf, policy, err := loadLockAndTrust(projRes.Root, lockPath, trustPath)
	if err != nil {
		return fail(err)
	}
	env.Lockfile = &lf

	cache, err := dist.OpenCache(deps.Env)
	if err != nil {
		return fail(err)
	}
	for _, entry := range lf.Kits {
		resolved, rErr := dist.ResolveLockedKit(ctx, deps.HTTPClient, cache, projRes.Root, entry, buildinfo.Version, offline, dist.CacheReadOnly)
		if rErr != nil {
			return fail(rErr)
		}
		if pErr := dist.VerifyProvenance(entry.ID, entry.Version, entry.LockTargetDigest(), entry.Provenance, policy, requireSig, deps.Now()); pErr != nil {
			return fail(pErr)
		}
		_ = resolved
	}

	env.Warnings = append(env.Warnings, "integrity hashes verify byte identity and Ed25519 signatures verify publisher key ownership; neither guarantees skill or hook prompt safety")
	if redact {
		env.Roots = cloneRootsMap(env.Roots)
		redactEnvelope(&env, collectHomeCandidates(deps))
	}
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintf(deps.Stdout, "Verified %d locked kit(s) cleanly (integrity and trust policy checks passed).\n", len(lf.Kits))
	return nil
}

func runLockDiff(ctx context.Context, args []string, deps Deps, env Envelope, fail func(error) error) error {
	var projectPath string
	var jsonOutput, redact bool
	var files []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--redact":
			redact = true
		case "--path":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a value"))
			}
			projectPath = args[i]
		case "--verbose":
			deps.Verbose = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown lock diff option %q", args[i])))
			}
			files = append(files, args[i])
		}
	}
	var oldPath, newPath string
	switch len(files) {
	case 2:
		oldPath, newPath = files[0], files[1]
	case 1:
		projRes, err := paths.ResolveProject(ctx, deps.Env, projectPath)
		if err != nil {
			return fail(err)
		}
		oldPath = filepath.Join(projRes.Root, dist.LockfileRelPath)
		newPath = files[0]
	default:
		return fail(apperr.Usage("missing_arguments", "usage: dtkit lock diff [<old-lock>] <new-lock> [--path <dir>] [--json]"))
	}

	oldRaw, err := readRegularFileBounded(oldPath, dist.MaxLockfileBytes)
	if err != nil {
		return fail(err)
	}
	oldLF, err := dist.ParseAndValidateLockfile(oldRaw)
	if err != nil {
		return fail(err)
	}
	newRaw, err := readRegularFileBounded(newPath, dist.MaxLockfileBytes)
	if err != nil {
		return fail(err)
	}
	newLF, err := dist.ParseAndValidateLockfile(newRaw)
	if err != nil {
		return fail(err)
	}
	report := dist.DiffLockfiles(oldLF, newLF)
	env.DryRun = true
	env.LockDiff = &report
	if redact {
		redactEnvelope(&env, collectHomeCandidates(deps))
	}
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	printLockDiffText(deps.Stdout, &report)
	return nil
}

func runLockSign(ctx context.Context, args []string, deps Deps, env Envelope, fail func(error) error) error {
	var projectPath, lockPath, keyFile, keyID, targetKit string
	var dryRun, yes, jsonOutput, redact bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run":
			dryRun = true
		case "--yes":
			yes = true
		case "--json":
			jsonOutput = true
		case "--redact":
			redact = true
		case "--verbose":
			deps.Verbose = true
		case "--path", "--lock", "--key-file", "--key-id", "--kit":
			flag := args[i]
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
			}
			switch flag {
			case "--path":
				projectPath = args[i]
			case "--lock":
				lockPath = args[i]
			case "--key-file":
				keyFile = args[i]
			case "--key-id":
				keyID = args[i]
			case "--kit":
				targetKit = args[i]
			}
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown lock sign option %q", args[i])))
		}
	}
	if keyFile == "" || keyID == "" {
		return fail(apperr.Usage("missing_signing_key", "--key-file and --key-id are required"))
	}
	projRes, err := paths.ResolveProject(ctx, deps.Env, projectPath)
	if err != nil {
		return fail(err)
	}
	resolvedLockPath := lockPath
	if resolvedLockPath == "" {
		resolvedLockPath = filepath.Join(projRes.Root, dist.LockfileRelPath)
	}
	rawLF, err := readRegularFileBounded(resolvedLockPath, dist.MaxLockfileBytes)
	if err != nil {
		return fail(err)
	}
	lf, err := dist.ParseAndValidateLockfile(rawLF)
	if err != nil {
		return fail(err)
	}
	rawKey, err := os.ReadFile(keyFile)
	if err != nil {
		return fail(apperr.WithPath(apperr.Usage("key_read_failed", "unable to read signing private key"), keyFile))
	}
	privB64 := strings.TrimSpace(string(rawKey))

	beforeLF := lf
	nextLF := dist.Lockfile{SchemaVersion: 1, Kits: make([]dist.LockedKit, len(lf.Kits))}
	copy(nextLF.Kits, lf.Kits)
	signedCount := 0
	for i := range nextLF.Kits {
		if targetKit != "" && nextLF.Kits[i].ID != targetKit {
			continue
		}
		prov, sErr := dist.SignDigest(nextLF.Kits[i].ID, nextLF.Kits[i].Version, nextLF.Kits[i].LockTargetDigest(), keyID, privB64)
		if sErr != nil {
			return fail(sErr)
		}
		nextLF.Kits[i].Provenance = &prov
		signedCount++
	}
	if signedCount == 0 {
		return fail(apperr.Usage("kit_not_in_lock", fmt.Sprintf("kit %q not found in lockfile", targetKit)))
	}
	outBytes, err := dist.MarshalLockfile(nextLF)
	if err != nil {
		return fail(err)
	}
	validated, err := dist.ParseAndValidateLockfile(outBytes)
	if err != nil {
		return fail(err)
	}
	diff := dist.DiffLockfiles(beforeLF, validated)
	env.DryRun = dryRun
	env.Lockfile = &validated
	env.LockDiff = &diff

	if dryRun {
		if redact {
			redactEnvelope(&env, collectHomeCandidates(deps))
		}
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Read-only preview: signed %d kit(s) with key %s.\n", signedCount, keyID)
		return nil
	}
	if jsonOutput && !yes {
		return fail(apperr.Usage("confirmation_required", "--json lock sign requires --yes"))
	}
	if !yes && !deps.IsInteractive() {
		return fail(apperr.Usage("confirmation_required", "non-interactive lock sign requires --yes"))
	}
	if err := atomicWriteFile(resolvedLockPath, outBytes, 0644); err != nil {
		return fail(err)
	}
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintf(deps.Stdout, "Signed %d kit(s) in %s using key %s.\n", signedCount, resolvedLockPath, keyID)
	return nil
}

func runLockKeygen(args []string, deps Deps, env Envelope, fail func(error) error) error {
	var keyID, publisher, outPrefix string
	var jsonOutput bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		case "--key-id", "--publisher", "--out-prefix":
			flag := args[i]
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
			}
			switch flag {
			case "--key-id":
				keyID = args[i]
			case "--publisher":
				publisher = args[i]
			case "--out-prefix":
				outPrefix = args[i]
			}
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown lock keygen option %q", args[i])))
		}
	}
	if keyID == "" || outPrefix == "" {
		return fail(apperr.Usage("missing_arguments", "usage: dtkit lock keygen --key-id <id> --out-prefix <path> [--publisher <name>] [--json]"))
	}
	if publisher == "" {
		publisher = keyID
	}
	pubB64, privB64, err := dist.GenerateEd25519Keypair()
	if err != nil {
		return fail(err)
	}
	policyBytes, err := dist.MarshalTrustPolicy(dist.TrustPolicy{
		SchemaVersion:    1,
		RequireSignature: true,
		Keys: []dist.TrustedKey{{
			KeyID:     keyID,
			Publisher: publisher,
			Algorithm: "ed25519",
			PublicKey: pubB64,
			Status:    "active",
		}},
	})
	if err != nil {
		return fail(err)
	}
	if _, err := dist.ParseAndValidateTrustPolicy(policyBytes); err != nil {
		return fail(err)
	}

	absPrefix, err := filepath.Abs(outPrefix)
	if err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(filepath.Dir(absPrefix), 0755); err != nil {
		return fail(err)
	}
	if err := atomicWriteFile(absPrefix+".pub.b64", []byte(pubB64+"\n"), 0644); err != nil {
		return fail(err)
	}
	if err := atomicWriteFile(absPrefix+".key.b64", []byte(privB64+"\n"), 0600); err != nil {
		return fail(err)
	}
	if err := atomicWriteFile(absPrefix+".trust.json", policyBytes, 0644); err != nil {
		return fail(err)
	}
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintf(deps.Stdout, "Generated Ed25519 keypair %s: %s.pub.b64, %s.key.b64, %s.trust.json\n", keyID, absPrefix, absPrefix, absPrefix)
	return nil
}

func loadLockAndTrust(projectRoot, lockPath, trustPath string) (dist.Lockfile, *dist.TrustPolicy, error) {
	resolvedLock := lockPath
	if resolvedLock == "" {
		resolvedLock = filepath.Join(projectRoot, dist.LockfileRelPath)
	}
	rawLock, err := readRegularFileBounded(resolvedLock, dist.MaxLockfileBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return dist.Lockfile{}, nil, apperr.WithPath(apperr.Usage("lockfile_not_found", "dtkit.lock does not exist"), resolvedLock)
		}
		return dist.Lockfile{}, nil, err
	}
	lf, err := dist.ParseAndValidateLockfile(rawLock)
	if err != nil {
		return dist.Lockfile{}, nil, err
	}

	var policy *dist.TrustPolicy
	if trustPath != "" {
		rawTrust, tErr := readRegularFileBounded(trustPath, dist.MaxTrustPolicyBytes)
		if tErr != nil {
			return dist.Lockfile{}, nil, tErr
		}
		pol, pErr := dist.ParseAndValidateTrustPolicy(rawTrust)
		if pErr != nil {
			return dist.Lockfile{}, nil, pErr
		}
		policy = &pol
	} else {
		defaultTrust := filepath.Join(projectRoot, filepath.FromSlash(dist.TrustPolicyRelPath))
		if rawTrust, tErr := readRegularFileBounded(defaultTrust, dist.MaxTrustPolicyBytes); tErr == nil {
			pol, pErr := dist.ParseAndValidateTrustPolicy(rawTrust)
			if pErr != nil {
				return dist.Lockfile{}, nil, pErr
			}
			policy = &pol
		} else if !os.IsNotExist(tErr) {
			return dist.Lockfile{}, nil, tErr
		}
	}
	return lf, policy, nil
}

func readRegularFileBounded(path string, maxBytes int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
		return nil, apperr.WithPath(apperr.Usage("invalid_file", "target must be a regular non-symlink file"), path)
	}
	if info.Size() > maxBytes {
		return nil, apperr.WithPath(apperr.Usage("file_too_large", fmt.Sprintf("file exceeds %d byte limit", maxBytes)), path)
	}
	return os.ReadFile(path)
}

func atomicWriteFile(target string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".dtkit-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(mode); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cErr := tmp.Close(); err == nil {
		err = cErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

func printLockDiffText(out interface{ Write([]byte) (int, error) }, diff *dist.LockDiffReport) {
	if diff == nil || len(diff.Changes) == 0 {
		fmt.Fprintln(out, "No lockfile changes.")
		return
	}
	for _, ch := range diff.Changes {
		fmt.Fprintf(out, "  [%s] %s: %s (%s) -> %s (%s) [src=%v ver=%v dig=%v sel=%v prov=%v]\n",
			ch.Status, ch.KitID, fallbackDash(ch.BeforeVersion), fallbackDash(ch.BeforeSource),
			fallbackDash(ch.AfterVersion), fallbackDash(ch.AfterSource),
			ch.SourceChanged, ch.VersionChanged, ch.DigestChanged, ch.SelectionChanged, ch.ProvenanceChanged)
	}
}
