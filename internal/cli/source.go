package cli

import (
	"context"
	"fmt"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/buildinfo"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func resolveKitSource(ctx context.Context, deps Deps, kitSpec, fromPath, sourceURL, sha256Hex string, offline bool, mode dist.CacheMode) (dist.ResolvedPackage, error) {
	var out dist.ResolvedPackage
	kitID, wantVersion, err := dist.ParseKitSpec(kitSpec)
	if err != nil {
		return out, err
	}
	if fromPath != "" && sourceURL != "" {
		return out, apperr.Usage("invalid_flag_combination", "--from and --source cannot be used together")
	}

	if fromPath != "" {
		res, err := dist.ResolveFromPath(fromPath, kitID, wantVersion, sha256Hex)
		if err != nil {
			return out, err
		}
		if err := dist.CheckMinCLIVersion(buildinfo.Version, res.Package.MinCLIVersion, res.Package.ID+" "+res.Package.Version); err != nil {
			return out, err
		}
		return res, nil
	}

	if sourceURL != "" || (offline && (sha256Hex != "" || kitID != "engineer")) {
		cache, err := dist.OpenCache(deps.Env)
		if err != nil {
			return out, err
		}
		return dist.ResolveRemote(ctx, deps.HTTPClient, cache, kitID, wantVersion, sourceURL, sha256Hex, buildinfo.Version, offline, mode)
	}

	if sha256Hex != "" {
		return out, apperr.Usage("invalid_flag_combination", "--sha256 requires --from <archive>, --source <url>, or --offline")
	}
	if kitID != "engineer" {
		return out, apperr.Usage("unknown_kit", fmt.Sprintf("kit %q is not in the bundled catalog (pass --from or --source)", kitID))
	}
	pkg, source, err := catalog.Engineer()
	if err != nil {
		return out, err
	}
	if wantVersion != "" && pkg.Version != wantVersion {
		return out, apperr.Usage("kit_version_mismatch", fmt.Sprintf("bundled %s version is %s, not %s", pkg.ID, pkg.Version, wantVersion))
	}
	dig, err := catalog.Digest(source)
	if err != nil {
		return out, err
	}
	if err := dist.CheckMinCLIVersion(buildinfo.Version, pkg.MinCLIVersion, pkg.ID+" "+pkg.Version); err != nil {
		return out, err
	}
	return dist.ResolvedPackage{
		Package:       pkg,
		Source:        source,
		PackageDigest: dig,
		SourceKind:    "bundled",
	}, nil
}

func persistResolvedCache(env paths.Environment, resolved dist.ResolvedPackage) error {
	if !resolved.HasPendingCacheWrite() {
		return nil
	}
	cache, err := dist.OpenCache(env)
	if err != nil {
		return err
	}
	_, err = resolved.PersistCache(cache)
	return err
}
