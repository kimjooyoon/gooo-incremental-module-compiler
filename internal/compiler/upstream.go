package compiler

import (
	"encoding/json"
	"fmt"
	"os"
)

type upstreamLock struct {
	Schema string `json:"schema"`
	Inputs []struct {
		Name       string `json:"name"`
		Repository string `json:"repository"`
		Tag        string `json:"tag"`
		Commit     string `json:"target_commit"`
		Immutable  bool   `json:"release_immutable"`
		Asset      struct {
			Digest string `json:"digest"`
		} `json:"asset"`
	} `json:"inputs"`
}

type linkedGraphInput struct {
	Schema          string `json:"schema"`
	Status          string `json:"status"`
	CanonicalDigest string `json:"canonical_digest"`
}

func ValidateOptionalInputs(lockPath, linkedGraphPath, runnerReleasePath string) ([]UpstreamInput, error) {
	var lock upstreamLock
	if err := loadJSON(lockPath, &lock); err != nil {
		return nil, err
	}
	if lock.Schema != "gooo/incremental-module-compiler/upstream-lock/v1" || len(lock.Inputs) != 3 {
		return nil, fmt.Errorf("upstream lock is not the v1 three-input contract")
	}
	byName := map[string]UpstreamInput{}
	for _, input := range lock.Inputs {
		if input.Name == "" || input.Repository == "" || input.Tag == "" || input.Commit == "" || !input.Immutable || input.Asset.Digest == "" {
			return nil, fmt.Errorf("upstream lock contains incomplete input %q", input.Name)
		}
		byName[input.Name] = UpstreamInput{Name: input.Name, Repository: input.Repository, Tag: input.Tag, Commit: input.Commit, Digest: input.Asset.Digest}
	}
	if _, ok := byName["canonical-linked-graph"]; !ok {
		return nil, fmt.Errorf("upstream lock omits canonical linked graph")
	}
	if _, ok := byName["public-causal-runner"]; !ok {
		return nil, fmt.Errorf("upstream lock omits public causal runner")
	}
	if linkedGraphPath != "" {
		var graph linkedGraphInput
		if err := loadJSON(linkedGraphPath, &graph); err != nil {
			return nil, err
		}
		if graph.Schema != "gooo.linked-ir/v1" || graph.Status != "CLOSED" || graph.CanonicalDigest == "" {
			return nil, fmt.Errorf("optional linked graph is not a closed canonical gooo.linked-ir/v1 record")
		}
		observedDigest, _, err := DigestFile(linkedGraphPath)
		if err != nil || observedDigest != byName["canonical-linked-graph"].Digest {
			return nil, fmt.Errorf("optional linked graph asset digest does not match the immutable release lock")
		}
	}
	if runnerReleasePath != "" {
		var release map[string]any
		if err := loadJSON(runnerReleasePath, &release); err != nil {
			return nil, err
		}
		immutable, ok := release["immutable"].(bool)
		if !ok {
			immutable, ok = release["release_immutable"].(bool)
		}
		if !ok || !immutable {
			return nil, fmt.Errorf("optional causal runner release is not platform immutable")
		}
		repository, repositoryOK := release["repository"].(string)
		tag, tagOK := release["tag"].(string)
		commit, commitOK := release["target_commit"].(string)
		assetDigest, assetDigestOK := release["asset_digest"].(string)
		if !repositoryOK || !tagOK || !commitOK || !assetDigestOK {
			return nil, fmt.Errorf("optional causal runner release must declare repository, tag, target_commit, and asset_digest")
		}
		if repository != byName["public-causal-runner"].Repository {
			return nil, fmt.Errorf("optional causal runner repository does not match the immutable release lock")
		}
		if tag != byName["public-causal-runner"].Tag {
			return nil, fmt.Errorf("optional causal runner tag does not match the immutable release lock")
		}
		if commit != byName["public-causal-runner"].Commit {
			return nil, fmt.Errorf("optional causal runner commit does not match the immutable release lock")
		}
		if assetDigest != byName["public-causal-runner"].Digest {
			return nil, fmt.Errorf("optional causal runner asset digest does not match the immutable release lock")
		}
	}
	result := []UpstreamInput{}
	if linkedGraphPath != "" {
		result = append(result, byName["canonical-linked-graph"])
	}
	if runnerReleasePath != "" {
		result = append(result, byName["public-causal-runner"])
	}
	return result, nil
}

func loadJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
