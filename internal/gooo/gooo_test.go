package gooo

import (
	"os"
	"path/filepath"
	"testing"
)

func testContract(t *testing.T) Contract {
	t.Helper()
	unknown := &UnknownRecord{Stage: "verify", Step: "observe", Reason: "not observable", UnknownClass: "external", NextOperation: "retry", BlockedBy: "api"}
	return Contract{
		Schema: "gooo.bootstrap/v1", Authority: "metacode", ContractID: "test",
		Policy: Policy{
			BootstrapCommit: BootstrapRule{Exactly: 1, Exception: "BOOTSTRAP_EXCEPTION"},
			PostBootstrapDirectMain: DirectRule{Exactly: 0, ViolationStatus: StatusRefuted},
			StatusPrecedence: []Status{StatusRefuted, StatusUnknown, StatusClosed},
			UnknownFields: []string{"stage", "step", "reason", "unknown_class", "next_operation", "blocked_by"},
			ApplyBoundary: ApplyBoundary{PlanBeforeApply: true, TargetRepoWritesBefore: 0, CallerOwnedOutputsOnly: true},
			ImprovementEvidence: ImprovementEvidence{SameInputDigestRequired: true, BeforeAfterIntegerPairRequired: true, MissingEvidenceStatus: StatusUnknown},
			ClosureGuards: []ClosureGuard{{Claim: "global_language_self_improvement", WithoutEvidenceStatus: StatusUnknown}, {Claim: "external_utility", WithoutEvidenceStatus: StatusUnknown}},
		},
		Inventory: InventorySpec{ExcludePaths: []string{"README.md"}},
		Measurements: MeasurementSpec{
			FileClasses: []string{"go", "gooo", "subdirectory", "regular_file"}, PhysicalLines: true,
			WallMS: []string{"build", "test", "conformance"}, PeakRSSKiB: []string{"build", "test", "conformance"},
			TestCounts: []string{"total", "executed", "reused", "failed", "unknown"}, GeneratedArtifacts: []string{"count", "bytes"},
		},
		CanonicalCases: []CanonicalCase{
			{ID: "normal", ExpectedStatus: StatusClosed, GitHubAPI: "observed", Ruleset: "observed"},
			{ID: "unknown-api-observability", ExpectedStatus: StatusUnknown, GitHubAPI: "insufficient", Ruleset: "insufficient", Unknown: unknown},
			{ID: "direct-main-observed", ExpectedStatus: StatusRefuted, PostBootstrapDirectMain: 1, GitHubAPI: "observed", Ruleset: "observed"},
		},
	}
}

func TestStatusPrecedence(t *testing.T) {
	contract := testContract(t)
	result := Evaluate(contract, Observation{BootstrapCommits: 1, BootstrapCommitsKnown: true, PostBootstrapDirectMain: 1, PostBootstrapDirectMainKnown: true, GitHubAPI: "insufficient", Ruleset: "insufficient"})
	if result.Status != StatusRefuted {
		t.Fatalf("got %s, want REFUTED", result.Status)
	}
}

func TestUnknownPreservesSixFields(t *testing.T) {
	contract := testContract(t)
	result := Evaluate(contract, Observation{BootstrapCommits: 1, BootstrapCommitsKnown: true, PostBootstrapDirectMainKnown: true, GitHubAPI: "insufficient", Ruleset: "insufficient"})
	if result.Status != StatusUnknown || result.Unknown == nil {
		t.Fatalf("got %#v, want UNKNOWN with record", result)
	}
	if err := result.Unknown.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSameInputDigestProducesSamePlan(t *testing.T) {
	contract := testContract(t)
	raw := []byte(`{"contract":"test"}`)
	first, err := BuildPlan(contract, raw, "sha256:input", "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildPlan(contract, raw, "sha256:input", "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if DigestJSON(first) != DigestJSON(second) || DigestJSON(first.Manifest) != DigestJSON(second.Manifest) || DigestJSON(first.Dossier) != DigestJSON(second.Dossier) {
		t.Fatal("same digest did not produce the same plan")
	}
}

func TestInventoryExcludesRootREADME(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	metrics, err := MeasureInventory(root, []string{"README.md"})
	if err != nil {
		t.Fatal(err)
	}
	if metrics.RegularFiles != 1 || metrics.GoFiles != 1 || metrics.GoPhysicalLines != 1 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
}

func TestCanonicalCases(t *testing.T) {
	contract := testContract(t)
	if err := ValidateCanonicalCases(contract); err != nil {
		t.Fatal(err)
	}
}
