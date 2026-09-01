package compiler

import (
	"path/filepath"
	"testing"
)

func TestPolicyAndModuleSourceAreExecutableInputs(t *testing.T) {
	policy, err := ParsePolicyFile(filepath.Join("..", "..", ".gooo", "incremental-compiler.gooo"))
	if err != nil {
		t.Fatal(err)
	}
	if policy.Denominator.Count != 12 || len(policy.Cells) != 12 || policy.Generation.Language != "go" {
		t.Fatalf("policy was not lowered from .gooo: %#v", policy)
	}
	module, err := ParseModuleFile(filepath.Join("..", "..", "fixtures", "conformance", "leaf-change", "core.gooo"))
	if err != nil {
		t.Fatal(err)
	}
	digest, err := ModuleSemanticDigest(module)
	if err != nil {
		t.Fatal(err)
	}
	if digest != module.SemanticDigest {
		t.Fatalf("semantic digest mismatch: got %s want %s", digest, module.SemanticDigest)
	}
}

func TestCanonicalCorpusDecisions(t *testing.T) {
	policy, err := ParsePolicyFile(filepath.Join("..", "..", ".gooo", "incremental-compiler.gooo"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Decision{
		"leaf-change":              Closed,
		"shared-dependency-change": Closed,
		"comment-only-nonsemantic": Closed,
		"missing-digest":           Unknown,
		"stale-cache":              Refuted,
	}
	for scenarioID, expected := range want {
		root := filepath.Join("..", "..", "fixtures", "conformance", scenarioID)
		modules, err := ParseModules([]string{filepath.Join(root, "core.gooo"), filepath.Join(root, "shared.gooo"), filepath.Join(root, "leaf.gooo")})
		if err != nil {
			t.Fatalf("%s modules: %v", scenarioID, err)
		}
		var input ScenarioInput
		if err := LoadJSON(filepath.Join(root, "scenario.json"), &input); err != nil {
			t.Fatalf("%s input: %v", scenarioID, err)
		}
		report, _, err := Evaluate(modules, input, policy)
		if err != nil {
			t.Fatalf("%s evaluate: %v", scenarioID, err)
		}
		if report.Decision != expected {
			t.Fatalf("%s decision=%s want=%s; unknowns=%#v refutations=%#v", scenarioID, report.Decision, expected, report.Unknowns, report.Refutations)
		}
		for _, detail := range report.Unknowns {
			if detail.Stage == "" || detail.Step == "" || detail.Reason == "" || detail.UnknownClass == "" || detail.NextOperation == "" || detail.BlockedBy == "" {
				t.Fatalf("%s has incomplete UNKNOWN tuple: %#v", scenarioID, detail)
			}
		}
	}
}
