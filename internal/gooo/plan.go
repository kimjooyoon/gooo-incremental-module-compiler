package gooo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Operation struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Mutation string `json:"mutation"`
}

type Manifest struct {
	Schema                      string      `json:"schema"`
	Authority                   string      `json:"authority"`
	InputDigest                 string      `json:"input_digest"`
	ContractDigest              string      `json:"contract_digest"`
	Target                      string      `json:"target"`
	TargetRepoWritesBeforeApply int         `json:"target_repo_writes_before_apply"`
	Operations                  []Operation `json:"operations"`
}

type Dossier struct {
	Schema                      string         `json:"schema"`
	Authority                   string         `json:"authority"`
	InputDigest                 string         `json:"input_digest"`
	ManifestDigest              string         `json:"manifest_digest"`
	Status                      Status         `json:"status"`
	Unknown                     *UnknownRecord `json:"unknown,omitempty"`
	TargetRepoWritesBeforeApply int            `json:"target_repo_writes_before_apply"`
}

type PlanArtifact struct {
	Schema   string   `json:"schema"`
	Manifest Manifest `json:"manifest"`
	Dossier  Dossier  `json:"dossier"`
}

func BuildPlan(contract Contract, contractRaw []byte, inputDigest, target string) (PlanArtifact, error) {
	if inputDigest == "" {
		inputDigest = Digest(contractRaw)
	}
	if target == "" {
		return PlanArtifact{}, errors.New("target is required")
	}
	manifest := Manifest{
		Schema:                      "gooo.manifest/v1",
		Authority:                   contract.Authority,
		InputDigest:                 inputDigest,
		ContractDigest:              Digest(contractRaw),
		Target:                      target,
		TargetRepoWritesBeforeApply: contract.Policy.ApplyBoundary.TargetRepoWritesBefore,
		Operations: []Operation{
			{ID: "bootstrap-commit", Kind: "git.commit", Mutation: "create exactly one BOOTSTRAP_EXCEPTION commit containing .gooo, Go 1.27 CI, PR-only verifier, and evidence upload"},
			{ID: "pull-request-only", Kind: "github.policy", Mutation: "require post-bootstrap changes through pull requests"},
			{ID: "evidence-artifact", Kind: "ci.artifact", Mutation: "upload deterministic verification and measurement evidence"},
		},
	}
	manifestDigest := DigestJSON(manifest)
	dossier := Dossier{
		Schema:                      "gooo.dossier/v1",
		Authority:                   contract.Authority,
		InputDigest:                 inputDigest,
		ManifestDigest:              manifestDigest,
		Status:                      StatusUnknown,
		Unknown:                     contract.unknownTemplate(),
		TargetRepoWritesBeforeApply: contract.Policy.ApplyBoundary.TargetRepoWritesBefore,
	}
	return PlanArtifact{Schema: "gooo.plan/v1", Manifest: manifest, Dossier: dossier}, nil
}

func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func DigestJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal deterministic JSON: %v", err))
	}
	return Digest(encoded)
}

func WriteCallerJSON(path, repoRoot string, value any) error {
	if path == "" {
		return errors.New("output path is required")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	absoluteRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(absoluteRoot, absolutePath)
	if err != nil {
		return err
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return errors.New("caller-owned output must be outside the target repository")
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absolutePath, append(encoded, '\n'), 0o644)
}
