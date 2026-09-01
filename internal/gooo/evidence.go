package gooo

import (
	"encoding/json"
	"errors"
	"os"
)

type VerifyReport struct {
	Schema         string      `json:"schema"`
	ContractDigest string      `json:"contract_digest"`
	InputDigest    string      `json:"input_digest"`
	Observation    Observation `json:"observation"`
	Result         Result      `json:"result"`
}

type Evidence struct {
	Schema                      string           `json:"schema"`
	Authority                   string           `json:"authority"`
	ContractDigest              string           `json:"contract_digest"`
	InputDigest                 string           `json:"input_digest"`
	PolicyStatus                Status           `json:"policy_status"`
	Unknown                     *UnknownRecord   `json:"unknown,omitempty"`
	Inventory                   InventoryMetrics `json:"inventory"`
	Runtime                     RuntimeMetrics   `json:"runtime"`
	Tests                       TestMetrics      `json:"tests"`
	GeneratedArtifacts          ArtifactMetrics  `json:"generated_artifacts"`
	TargetRepoWritesBeforeApply int              `json:"target_repo_writes_before_apply"`
}

func LoadVerifyReport(path string) (VerifyReport, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return VerifyReport{}, err
	}
	var report VerifyReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return VerifyReport{}, err
	}
	if report.Result.Status == "" {
		return VerifyReport{}, errors.New("verification report has no status")
	}
	return report, nil
}
