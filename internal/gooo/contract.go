package gooo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type Status string

const (
	StatusClosed  Status = "CLOSED"
	StatusUnknown Status = "UNKNOWN"
	StatusRefuted Status = "REFUTED"
)

type Contract struct {
	Schema          string             `json:"schema"`
	Authority       string             `json:"authority"`
	ContractID      string             `json:"contract_id"`
	Policy          Policy             `json:"policy"`
	Inventory       InventorySpec      `json:"inventory"`
	Measurements    MeasurementSpec    `json:"measurements"`
	CanonicalCases  []CanonicalCase    `json:"canonical_cases"`
}

type Policy struct {
	BootstrapCommit         BootstrapRule `json:"bootstrap_commit"`
	PostBootstrapDirectMain DirectRule    `json:"post_bootstrap_direct_main"`
	StatusPrecedence        []Status      `json:"status_precedence"`
	UnknownFields           []string      `json:"unknown_fields"`
	ApplyBoundary           ApplyBoundary `json:"apply_boundary"`
	ImprovementEvidence     ImprovementEvidence `json:"improvement_evidence"`
	ClosureGuards           []ClosureGuard `json:"closure_guards"`
}

type BootstrapRule struct {
	Exactly   int    `json:"exactly"`
	Exception string `json:"exception"`
}

type DirectRule struct {
	Exactly         int    `json:"exactly"`
	ViolationStatus Status `json:"violation_status"`
}

type ApplyBoundary struct {
	PlanBeforeApply           bool `json:"plan_before_apply"`
	TargetRepoWritesBefore   int  `json:"target_repo_writes_before_apply"`
	CallerOwnedOutputsOnly   bool `json:"caller_owned_outputs_only"`
}

type ImprovementEvidence struct {
	SameInputDigestRequired       bool   `json:"same_input_digest_required"`
	BeforeAfterIntegerPairRequired bool  `json:"before_after_integer_pair_required"`
	MissingEvidenceStatus         Status `json:"missing_evidence_status"`
}

type ClosureGuard struct {
	Claim                string `json:"claim"`
	WithoutEvidenceStatus Status `json:"without_evidence_status"`
}

type InventorySpec struct {
	ExcludePaths []string `json:"exclude_paths"`
}

type MeasurementSpec struct {
	FileClasses        []string `json:"file_classes"`
	PhysicalLines      bool     `json:"physical_lines"`
	WallMS             []string `json:"wall_ms"`
	PeakRSSKiB         []string `json:"peak_rss_kib"`
	TestCounts         []string `json:"test_counts"`
	GeneratedArtifacts []string `json:"generated_artifacts"`
}

type CanonicalCase struct {
	ID                       string         `json:"id"`
	ExpectedStatus           Status         `json:"expected_status"`
	PostBootstrapDirectMain int            `json:"post_bootstrap_direct_main"`
	GitHubAPI                string         `json:"github_api"`
	Ruleset                  string         `json:"ruleset"`
	Unknown                  *UnknownRecord `json:"unknown,omitempty"`
}

type UnknownRecord struct {
	Stage         string `json:"stage"`
	Step          string `json:"step"`
	Reason        string `json:"reason"`
	UnknownClass  string `json:"unknown_class"`
	NextOperation string `json:"next_operation"`
	BlockedBy     string `json:"blocked_by"`
}

type Observation struct {
	BootstrapCommits            int    `json:"bootstrap_commits"`
	BootstrapCommitsKnown       bool   `json:"bootstrap_commits_known"`
	PostBootstrapDirectMain     int    `json:"post_bootstrap_direct_main"`
	PostBootstrapDirectMainKnown bool  `json:"post_bootstrap_direct_main_known"`
	GitHubAPI                   string `json:"github_api"`
	Ruleset                     string `json:"ruleset"`
}

type Result struct {
	Status  Status         `json:"status"`
	Reason  string         `json:"reason,omitempty"`
	Unknown *UnknownRecord `json:"unknown,omitempty"`
}

func LoadContract(path string) (Contract, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Contract{}, nil, err
	}
	var contract Contract
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&contract); err != nil {
		return Contract{}, nil, fmt.Errorf("parse .gooo contract: %w", err)
	}
	if err := contract.Validate(); err != nil {
		return Contract{}, nil, err
	}
	return contract, raw, nil
}

func (c Contract) Validate() error {
	if c.Schema != "gooo.bootstrap/v1" {
		return fmt.Errorf("unsupported contract schema %q", c.Schema)
	}
	if c.Authority != "metacode" {
		return errors.New(".gooo metacode must be authoritative")
	}
	if c.ContractID == "" {
		return errors.New("contract_id is required")
	}
	if c.Policy.BootstrapCommit.Exactly != 1 || c.Policy.BootstrapCommit.Exception != "BOOTSTRAP_EXCEPTION" {
		return errors.New("bootstrap commit must be exactly one BOOTSTRAP_EXCEPTION")
	}
	if c.Policy.PostBootstrapDirectMain.Exactly != 0 || c.Policy.PostBootstrapDirectMain.ViolationStatus != StatusRefuted {
		return errors.New("post-bootstrap direct-main must be exactly zero with REFUTED violation")
	}
	if !equalStrings(c.Policy.StatusPrecedence, []Status{StatusRefuted, StatusUnknown, StatusClosed}) {
		return errors.New("status precedence must be REFUTED > UNKNOWN > CLOSED")
	}
	if !equalStrings(c.Policy.UnknownFields, []StatusString{
		"stage", "step", "reason", "unknown_class", "next_operation", "blocked_by",
	}) {
		return errors.New("unknown_fields must contain the required six fields in order")
	}
	if !c.Policy.ApplyBoundary.PlanBeforeApply || c.Policy.ApplyBoundary.TargetRepoWritesBefore != 0 || !c.Policy.ApplyBoundary.CallerOwnedOutputsOnly {
		return errors.New("apply boundary must plan first, write zero target files before apply, and use caller-owned outputs")
	}
	if !c.Policy.ImprovementEvidence.SameInputDigestRequired || !c.Policy.ImprovementEvidence.BeforeAfterIntegerPairRequired || c.Policy.ImprovementEvidence.MissingEvidenceStatus != StatusUnknown {
		return errors.New("improvement claims require same-digest integer before/after evidence or UNKNOWN")
	}
	if len(c.Policy.ClosureGuards) != 2 || c.Policy.ClosureGuards[0].Claim != "global_language_self_improvement" || c.Policy.ClosureGuards[1].Claim != "external_utility" || c.Policy.ClosureGuards[0].WithoutEvidenceStatus != StatusUnknown || c.Policy.ClosureGuards[1].WithoutEvidenceStatus != StatusUnknown {
		return errors.New("global language self-improvement and external utility require evidence before closure")
	}
	if len(c.Inventory.ExcludePaths) != 1 || c.Inventory.ExcludePaths[0] != "README.md" {
		return errors.New("inventory must exclude root README.md")
	}
	if !equalStrings(c.Measurements.FileClasses, []StatusString{"go", "gooo", "subdirectory", "regular_file"}) || !c.Measurements.PhysicalLines {
		return errors.New("inventory measurement classes or physical line measurement are incomplete")
	}
	if !equalStrings(c.Measurements.WallMS, []StatusString{"build", "test", "conformance"}) || !equalStrings(c.Measurements.PeakRSSKiB, []StatusString{"build", "test", "conformance"}) {
		return errors.New("runtime measurement stages are incomplete")
	}
	if !equalStrings(c.Measurements.TestCounts, []StatusString{"total", "executed", "reused", "failed", "unknown"}) || !equalStrings(c.Measurements.GeneratedArtifacts, []StatusString{"count", "bytes"}) {
		return errors.New("test or artifact measurement fields are incomplete")
	}
	if len(c.CanonicalCases) != 3 {
		return errors.New("exactly three canonical cases are required")
	}
	seen := map[string]bool{}
	for _, canonical := range c.CanonicalCases {
		if seen[canonical.ID] {
			return fmt.Errorf("duplicate canonical case %q", canonical.ID)
		}
		seen[canonical.ID] = true
		if err := validateCanonicalCase(canonical); err != nil {
			return err
		}
	}
	for _, id := range []string{"normal", "unknown-api-observability", "direct-main-observed"} {
		if !seen[id] {
			return fmt.Errorf("missing canonical case %q", id)
		}
	}
	return nil
}

type StatusString = string

func equalStrings[T ~string](actual []T, expected []T) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i := range actual {
		if string(actual[i]) != string(expected[i]) {
			return false
		}
	}
	return true
}

func validateCanonicalCase(canonical CanonicalCase) error {
	var expected Status
	switch canonical.ID {
	case "normal":
		expected = StatusClosed
		if canonical.PostBootstrapDirectMain != 0 || canonical.GitHubAPI != "observed" || canonical.Ruleset != "observed" || canonical.Unknown != nil {
			return errors.New("normal canonical case is malformed")
		}
	case "unknown-api-observability":
		expected = StatusUnknown
		if canonical.PostBootstrapDirectMain != 0 || canonical.Unknown == nil {
			return errors.New("unknown canonical case is malformed")
		}
		if err := canonical.Unknown.Validate(); err != nil {
			return err
		}
	case "direct-main-observed":
		expected = StatusRefuted
		if canonical.PostBootstrapDirectMain != 1 || canonical.Unknown != nil {
			return errors.New("refuted canonical case is malformed")
		}
	default:
		return fmt.Errorf("unexpected canonical case %q", canonical.ID)
	}
	if canonical.ExpectedStatus != expected {
		return fmt.Errorf("canonical case %q expected status is %q, got %q", canonical.ID, expected, canonical.ExpectedStatus)
	}
	return nil
}

func (u UnknownRecord) Validate() error {
	if u.Stage == "" || u.Step == "" || u.Reason == "" || u.UnknownClass == "" || u.NextOperation == "" || u.BlockedBy == "" {
		return errors.New("UNKNOWN must preserve stage, step, reason, unknown_class, next_operation, and blocked_by")
	}
	return nil
}

func (c Contract) unknownTemplate() *UnknownRecord {
	for _, canonical := range c.CanonicalCases {
		if canonical.ExpectedStatus == StatusUnknown && canonical.Unknown != nil {
			copy := *canonical.Unknown
			return &copy
		}
	}
	return nil
}

func Evaluate(c Contract, observation Observation) Result {
	if observation.PostBootstrapDirectMainKnown && observation.PostBootstrapDirectMain > 0 {
		return Result{Status: StatusRefuted, Reason: "post-bootstrap direct-main commit observed"}
	}
	if observation.BootstrapCommitsKnown && observation.BootstrapCommits != 1 {
		return Result{Status: StatusRefuted, Reason: "bootstrap commit count is not exactly one"}
	}
	if !observation.BootstrapCommitsKnown || !observation.PostBootstrapDirectMainKnown || observation.GitHubAPI != "observed" || observation.Ruleset != "observed" {
		return Result{Status: StatusUnknown, Unknown: c.unknownTemplate()}
	}
	return Result{Status: StatusClosed, Reason: "bootstrap and post-bootstrap direct-main policy observed"}
}

func ValidateCanonicalCases(c Contract) error {
	for _, canonical := range c.CanonicalCases {
		observation := Observation{
			BootstrapCommits:             1,
			BootstrapCommitsKnown:        true,
			PostBootstrapDirectMain:      canonical.PostBootstrapDirectMain,
			PostBootstrapDirectMainKnown: true,
			GitHubAPI:                    canonical.GitHubAPI,
			Ruleset:                      canonical.Ruleset,
		}
		result := Evaluate(c, observation)
		if result.Status != canonical.ExpectedStatus {
			return fmt.Errorf("canonical case %q evaluated as %q, expected %q", canonical.ID, result.Status, canonical.ExpectedStatus)
		}
		if result.Status == StatusUnknown && (result.Unknown == nil || result.Unknown.Validate() != nil) {
			return fmt.Errorf("canonical case %q lost UNKNOWN six-field record", canonical.ID)
		}
	}
	return nil
}
