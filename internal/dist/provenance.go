package dist

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

const (
	// TrustPolicyRelPath is the default project-local publisher trust root configuration.
	TrustPolicyRelPath = ".dtkit/trust.json"
	// MaxTrustPolicyBytes bounds trust policy files.
	MaxTrustPolicyBytes = 256 * 1024
)

var keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ProvenanceRecord attaches a detached Ed25519 publisher signature over a verified kit digest.
// SHA-256 verifies byte integrity and Ed25519 verifies publisher identity under trust policy;
// neither proves prompt, skill, or hook behavioral safety on its own.
type ProvenanceRecord struct {
	KeyID        string `json:"keyId"`
	Algorithm    string `json:"algorithm"`
	SignedDigest string `json:"signedDigest"`
	Signature    string `json:"signature"`
}

// TrustedKey defines a single publisher public key in TrustPolicy, supporting rotation and revocation.
type TrustedKey struct {
	KeyID       string   `json:"keyId"`
	Publisher   string   `json:"publisher"`
	Algorithm   string   `json:"algorithm"`
	PublicKey   string   `json:"publicKey"`
	Status      string   `json:"status"` // "active" | "revoked"
	NotAfter    string   `json:"notAfter,omitempty"`
	AllowedKits []string `json:"allowedKits,omitempty"`
}

// TrustPolicy declares trusted publisher keys and signature enforcement for a project.
type TrustPolicy struct {
	SchemaVersion    int          `json:"schemaVersion"`
	RequireSignature bool         `json:"requireSignature"`
	Keys             []TrustedKey `json:"keys"`
}

// GenerateEd25519Keypair creates a fresh Ed25519 keypair encoded as standard base64 strings.
func GenerateEd25519Keypair() (publicB64, privateB64 string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(pub), base64.StdEncoding.EncodeToString(priv), nil
}

// CanonicalProvenanceMessage builds the exact domain-separated bytes signed by publisher keys.
func CanonicalProvenanceMessage(kitID, version, targetDigest string) []byte {
	return []byte(fmt.Sprintf("dtkit-provenance-v1\n%s\n%s\n%s\n", kitID, version, strings.ToLower(targetDigest)))
}

// SignDigest produces a ProvenanceRecord using a standard base64-encoded Ed25519 private key.
func SignDigest(kitID, version, targetDigest, keyID, privateKeyB64 string) (ProvenanceRecord, error) {
	var rec ProvenanceRecord
	if !catalog.IsValidID(kitID, 32) {
		return rec, apperr.Usage("invalid_kit_id", fmt.Sprintf("invalid kit id %q", kitID))
	}
	if !catalog.IsValidVersion(version) {
		return rec, apperr.Usage("invalid_version", fmt.Sprintf("invalid kit version %q", version))
	}
	normDigest, err := normalizeSHA256(targetDigest)
	if err != nil {
		return rec, err
	}
	if !keyIDPattern.MatchString(keyID) {
		return rec, apperr.Usage("invalid_key_id", fmt.Sprintf("invalid key id %q", keyID))
	}
	privBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privateKeyB64))
	if err != nil || len(privBytes) != ed25519.PrivateKeySize {
		return rec, apperr.Usage("invalid_private_key", "private key must be base64-encoded 64-byte Ed25519 key")
	}
	msg := CanonicalProvenanceMessage(kitID, version, normDigest)
	sig := ed25519.Sign(ed25519.PrivateKey(privBytes), msg)
	return ProvenanceRecord{
		KeyID:        keyID,
		Algorithm:    "ed25519",
		SignedDigest: normDigest,
		Signature:    base64.StdEncoding.EncodeToString(sig),
	}, nil
}

// ParseAndValidateTrustPolicy strictly parses and validates .dtkit/trust.json bytes.
func ParseAndValidateTrustPolicy(data []byte) (TrustPolicy, error) {
	var policy TrustPolicy
	if len(data) > MaxTrustPolicyBytes {
		return policy, apperr.Usage("trust_policy_too_large", fmt.Sprintf("trust policy exceeds %d byte limit", MaxTrustPolicyBytes))
	}
	if err := catalog.CheckStrictJSON(data); err != nil {
		return policy, apperr.Wrap(apperr.ExitUsage, "invalid_trust_policy", fmt.Sprintf("trust policy JSON error: %v", err), err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return policy, apperr.Wrap(apperr.ExitUsage, "invalid_trust_policy", "trust policy must be a JSON object", err)
	}
	if err := catalog.CheckObjectKeys(top, []string{"schemaVersion", "requireSignature", "keys"}, nil); err != nil {
		return policy, apperr.Usage("invalid_trust_policy", fmt.Sprintf("trust policy schema: %v", err))
	}
	var rawKeys []map[string]json.RawMessage
	if err := json.Unmarshal(top["keys"], &rawKeys); err != nil {
		return policy, apperr.Usage("invalid_trust_policy", "trust policy keys must be an array of objects")
	}
	for i, rk := range rawKeys {
		if err := catalog.CheckObjectKeys(rk, []string{"keyId", "publisher", "algorithm", "publicKey", "status"}, []string{"notAfter", "allowedKits"}); err != nil {
			return policy, apperr.Usage("invalid_trust_policy", fmt.Sprintf("trust key %d: %v", i, err))
		}
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&policy); err != nil || policy.SchemaVersion != 1 || policy.Keys == nil {
		return TrustPolicy{}, apperr.Usage("invalid_trust_policy", "trust policy must be valid JSON with schemaVersion 1")
	}

	seenKeys := make(map[string]struct{}, len(policy.Keys))
	for i, k := range policy.Keys {
		if !keyIDPattern.MatchString(k.KeyID) {
			return TrustPolicy{}, apperr.Usage("invalid_trust_policy", fmt.Sprintf("trust key %d has invalid keyId %q", i, k.KeyID))
		}
		if _, dup := seenKeys[k.KeyID]; dup {
			return TrustPolicy{}, apperr.Usage("duplicate_key_id", fmt.Sprintf("duplicate trusted keyId %q", k.KeyID))
		}
		seenKeys[k.KeyID] = struct{}{}
		if strings.TrimSpace(k.Publisher) == "" {
			return TrustPolicy{}, apperr.Usage("invalid_trust_policy", fmt.Sprintf("trust key %q missing publisher", k.KeyID))
		}
		if k.Algorithm != "ed25519" {
			return TrustPolicy{}, apperr.Usage("unsupported_key_algorithm", fmt.Sprintf("trust key %q uses unsupported algorithm %q", k.KeyID, k.Algorithm))
		}
		pubBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(k.PublicKey))
		if err != nil || len(pubBytes) != ed25519.PublicKeySize {
			return TrustPolicy{}, apperr.Usage("invalid_trust_policy", fmt.Sprintf("trust key %q has invalid base64 Ed25519 publicKey", k.KeyID))
		}
		if k.Status != "active" && k.Status != "revoked" {
			return TrustPolicy{}, apperr.Usage("invalid_trust_policy", fmt.Sprintf("trust key %q has invalid status %q", k.KeyID, k.Status))
		}
		if k.NotAfter != "" {
			if _, err := time.Parse(time.RFC3339, k.NotAfter); err != nil {
				return TrustPolicy{}, apperr.Usage("invalid_trust_policy", fmt.Sprintf("trust key %q has invalid RFC3339 notAfter %q", k.KeyID, k.NotAfter))
			}
		}
		for _, allowed := range k.AllowedKits {
			if !catalog.IsValidID(allowed, 32) {
				return TrustPolicy{}, apperr.Usage("invalid_trust_policy", fmt.Sprintf("trust key %q has invalid allowedKits entry %q", k.KeyID, allowed))
			}
		}
	}
	return policy, nil
}

// MarshalTrustPolicy serializes policy deterministically with sorted keys.
func MarshalTrustPolicy(policy TrustPolicy) ([]byte, error) {
	copyPol := policy
	copyPol.Keys = append([]TrustedKey(nil), policy.Keys...)
	if copyPol.Keys == nil {
		copyPol.Keys = []TrustedKey{}
	}
	sort.Slice(copyPol.Keys, func(i, j int) bool {
		return copyPol.Keys[i].KeyID < copyPol.Keys[j].KeyID
	})
	b, err := json.MarshalIndent(copyPol, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// VerifyProvenance checks publisher signature and key revocation/expiration against policy before mutation.
func VerifyProvenance(kitID, version, targetDigest string, prov *ProvenanceRecord, policy *TrustPolicy, forceRequireSig bool, now time.Time) error {
	requireSig := forceRequireSig || (policy != nil && policy.RequireSignature)
	if prov == nil {
		if requireSig {
			return apperr.WithHint(
				apperr.Conflict("signature_required", fmt.Sprintf("kit %s %s has no publisher provenance signature", kitID, version)),
				"sign the lockfile entry with an active trusted key or relax requireSignature in trust policy",
			)
		}
		return nil
	}
	if policy == nil {
		return apperr.WithHint(
			apperr.Conflict("missing_trust_policy", fmt.Sprintf("kit %s carries provenance signed by %q, but no trust policy (.dtkit/trust.json) is configured", kitID, prov.KeyID)),
			"provide .dtkit/trust.json or --trust <file> to verify publisher signatures",
		)
	}

	if prov.Algorithm != "ed25519" {
		return apperr.Conflict("unsupported_signature_algorithm", fmt.Sprintf("unsupported provenance algorithm %q", prov.Algorithm))
	}
	normTarget, err := normalizeSHA256(targetDigest)
	if err != nil {
		return apperr.Wrap(apperr.ExitConflict, "invalid_provenance_digest", "invalid target digest for provenance verification", err)
	}
	if !strings.EqualFold(prov.SignedDigest, normTarget) {
		return apperr.Conflict("provenance_digest_mismatch", fmt.Sprintf("provenance signedDigest %s does not match verified digest %s", prov.SignedDigest, normTarget))
	}

	var matched *TrustedKey
	for i := range policy.Keys {
		if policy.Keys[i].KeyID == prov.KeyID {
			matched = &policy.Keys[i]
			break
		}
	}
	if matched == nil {
		return apperr.Conflict("untrusted_key", fmt.Sprintf("provenance keyId %q is not in trust policy", prov.KeyID))
	}
	if matched.Status == "revoked" {
		return apperr.Conflict("key_revoked", fmt.Sprintf("provenance keyId %q has been revoked", prov.KeyID))
	}
	if matched.NotAfter != "" {
		exp, err := time.Parse(time.RFC3339, matched.NotAfter)
		if err != nil || now.After(exp) {
			return apperr.Conflict("key_expired", fmt.Sprintf("provenance keyId %q expired at %s", prov.KeyID, matched.NotAfter))
		}
	}
	if len(matched.AllowedKits) > 0 {
		allowed := false
		for _, k := range matched.AllowedKits {
			if k == kitID {
				allowed = true
				break
			}
		}
		if !allowed {
			return apperr.Conflict("key_kit_disallowed", fmt.Sprintf("trust key %q is not authorized to sign kit %q", matched.KeyID, kitID))
		}
	}

	pubBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(matched.PublicKey))
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return apperr.Conflict("invalid_trusted_key", fmt.Sprintf("trusted key %q is malformed", matched.KeyID))
	}
	sigBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(prov.Signature))
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return apperr.Conflict("invalid_signature", fmt.Sprintf("provenance signature for %s is malformed", kitID))
	}
	msg := CanonicalProvenanceMessage(kitID, version, normTarget)
	if !ed25519.Verify(ed25519.PublicKey(pubBytes), msg, sigBytes) {
		return apperr.Conflict("invalid_signature", fmt.Sprintf("Ed25519 provenance signature verification failed for %s (%s)", kitID, prov.KeyID))
	}
	return nil
}
