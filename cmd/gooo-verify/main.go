package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kimjooyoon/gooo-repository-bootstrap/internal/gooo"
)

func main() {
	if len(os.Args) < 2 {
		fail(errors.New("command is required: verify, plan, conformance, or evidence"))
	}
	switch os.Args[1] {
	case "verify":
		runVerify(os.Args[2:])
	case "plan":
		runPlan(os.Args[2:])
	case "conformance":
		runConformance(os.Args[2:])
	case "evidence":
		runEvidence(os.Args[2:])
	default:
		fail(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func runVerify(args []string) {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	contractPath := flags.String("contract", ".gooo/bootstrap.gooo", "authoritative .gooo contract")
	outputPath := flags.String("output", "", "caller-owned verification report")
	inputDigest := flags.String("input-digest", "", "stable input digest")
	observe := flags.Bool("observe-github", false, "read GitHub REST evidence")
	githubRepo := flags.String("repo", os.Getenv("GITHUB_REPOSITORY"), "owner/name repository")
	event := flags.String("event", os.Getenv("GITHUB_EVENT_NAME"), "workflow event")
	ref := flags.String("ref", os.Getenv("GITHUB_REF"), "workflow ref")
	sha := flags.String("sha", os.Getenv("GITHUB_SHA"), "commit SHA")
	before := flags.String("before", os.Getenv("GITHUB_EVENT_BEFORE"), "previous commit SHA")
	bootstrapCommits := flags.Int("bootstrap-commits", 1, "observed bootstrap commit count")
	bootstrapKnown := flags.Bool("bootstrap-known", true, "whether bootstrap count is observed")
	directMain := flags.Int("post-bootstrap-direct-main", -1, "observed direct-main count; -1 means unknown")
	githubAPI := flags.String("github-api", "insufficient", "observed or insufficient")
	ruleset := flags.String("ruleset", "insufficient", "observed or insufficient")
	allowUnknown := flags.Bool("allow-unknown", false, "write UNKNOWN evidence without failing the collection job")
	if err := flags.Parse(args); err != nil {
		fail(err)
	}
	contract, raw, err := gooo.LoadContract(*contractPath)
	if err != nil {
		fail(err)
	}
	observation := gooo.Observation{
		BootstrapCommits:             *bootstrapCommits,
		BootstrapCommitsKnown:        *bootstrapKnown,
		GitHubAPI:                    *githubAPI,
		Ruleset:                      *ruleset,
		PostBootstrapDirectMainKnown: *directMain >= 0,
	}
	if *directMain >= 0 {
		observation.PostBootstrapDirectMain = *directMain
	}
	if *observe {
		observation, err = gooo.ObserveGitHub(context.Background(), *githubRepo, *sha, *event, *ref, *before)
		if err != nil && observation.GitHubAPI == "" {
			observation.GitHubAPI = "insufficient"
		}
	}
	result := gooo.Evaluate(contract, observation)
	report := gooo.VerifyReport{
		Schema:         "gooo.verify-report/v1",
		ContractDigest: gooo.Digest(raw),
		InputDigest:    valueOr(*inputDigest, gooo.Digest(raw)),
		Observation:    observation,
		Result:         result,
	}
	writeReport(*outputPath, ".", report)
	if result.Status != gooo.StatusClosed && !(result.Status == gooo.StatusUnknown && *allowUnknown) {
		fail(fmt.Errorf("policy status is %s", result.Status))
	}
}

func runPlan(args []string) {
	flags := flag.NewFlagSet("plan", flag.ContinueOnError)
	contractPath := flags.String("contract", ".gooo/bootstrap.gooo", "authoritative .gooo contract")
	inputDigest := flags.String("input-digest", "", "stable input digest")
	target := flags.String("target", "", "owner/name repository")
	outputPath := flags.String("output", "", "caller-owned plan output")
	repoRoot := flags.String("repo-root", ".", "target repository root")
	if err := flags.Parse(args); err != nil {
		fail(err)
	}
	contract, raw, err := gooo.LoadContract(*contractPath)
	if err != nil {
		fail(err)
	}
	plan, err := gooo.BuildPlan(contract, raw, *inputDigest, *target)
	if err != nil {
		fail(err)
	}
	if err := gooo.WriteCallerJSON(*outputPath, *repoRoot, plan); err != nil {
		fail(err)
	}
}

func runConformance(args []string) {
	flags := flag.NewFlagSet("conformance", flag.ContinueOnError)
	contractPath := flags.String("contract", ".gooo/bootstrap.gooo", "authoritative .gooo contract")
	inputDigest := flags.String("input-digest", "conformance-input", "stable conformance input digest")
	target := flags.String("target", "example/repository", "deterministic target name")
	outputPath := flags.String("output", "", "caller-owned conformance report")
	repoRoot := flags.String("repo-root", ".", "target repository root")
	if err := flags.Parse(args); err != nil {
		fail(err)
	}
	contract, raw, err := gooo.LoadContract(*contractPath)
	if err != nil {
		fail(err)
	}
	if err := gooo.ValidateCanonicalCases(contract); err != nil {
		fail(err)
	}
	first, err := gooo.BuildPlan(contract, raw, *inputDigest, *target)
	if err != nil {
		fail(err)
	}
	second, err := gooo.BuildPlan(contract, raw, *inputDigest, *target)
	if err != nil {
		fail(err)
	}
	firstBytes, _ := json.Marshal(first)
	secondBytes, _ := json.Marshal(second)
	if string(firstBytes) != string(secondBytes) {
		fail(errors.New("same input digest did not produce identical manifest and dossier"))
	}
	report := struct {
		Schema     string `json:"schema"`
		Status     string `json:"status"`
		Precedence []string `json:"precedence"`
		Cases      int    `json:"canonical_cases"`
	}{
		Schema:     "gooo.conformance/v1",
		Status:     "PASS",
		Precedence: []string{"REFUTED", "UNKNOWN", "CLOSED"},
		Cases:      len(contract.CanonicalCases),
	}
	writeReport(*outputPath, *repoRoot, report)
}

func runEvidence(args []string) {
	flags := flag.NewFlagSet("evidence", flag.ContinueOnError)
	contractPath := flags.String("contract", ".gooo/bootstrap.gooo", "authoritative .gooo contract")
	policyPath := flags.String("policy", "", "verification report")
	inputDigest := flags.String("input-digest", "", "stable input digest")
	repoRoot := flags.String("repo-root", ".", "target repository root")
	outputPath := flags.String("output", "", "caller-owned evidence output")
	artifactDir := flags.String("artifact-dir", "", "generated artifact directory")
	buildTime := flags.String("build-time", "", "build runtime metric")
	testTime := flags.String("test-time", "", "test runtime metric")
	conformanceTime := flags.String("conformance-time", "", "conformance runtime metric")
	testJSON := flags.String("test-json", "", "go test -json output")
	if err := flags.Parse(args); err != nil {
		fail(err)
	}
	contract, raw, err := gooo.LoadContract(*contractPath)
	if err != nil {
		fail(err)
	}
	policy, err := gooo.LoadVerifyReport(*policyPath)
	if err != nil {
		fail(err)
	}
	inventory, err := gooo.MeasureInventory(*repoRoot, contract.Inventory.ExcludePaths)
	if err != nil {
		fail(err)
	}
	buildMetric, err := gooo.ParseRuntimeMetric(*buildTime)
	if err != nil {
		fail(err)
	}
	testMetric, err := gooo.ParseRuntimeMetric(*testTime)
	if err != nil {
		fail(err)
	}
	conformanceMetric, err := gooo.ParseRuntimeMetric(*conformanceTime)
	if err != nil {
		fail(err)
	}
	tests, err := gooo.ParseGoTestJSON(*testJSON)
	if err != nil {
		fail(err)
	}
	artifacts, err := gooo.MeasureArtifacts(*artifactDir)
	if err != nil {
		fail(err)
	}
	evidence := gooo.Evidence{
		Schema:                      "gooo.evidence/v1",
		Authority:                   contract.Authority,
		ContractDigest:              gooo.Digest(raw),
		InputDigest:                 valueOr(*inputDigest, policy.InputDigest),
		PolicyStatus:                policy.Result.Status,
		Unknown:                     policy.Result.Unknown,
		Inventory:                   inventory,
		Runtime:                     gooo.RuntimeMetrics{Build: buildMetric, Test: testMetric, Conformance: conformanceMetric},
		Tests:                       tests,
		GeneratedArtifacts:          artifacts,
		TargetRepoWritesBeforeApply: contract.Policy.ApplyBoundary.TargetRepoWritesBefore,
	}
	writeReport(*outputPath, *repoRoot, evidence)
}

func writeReport(path, repoRoot string, value any) {
	if err := gooo.WriteCallerJSON(path, repoRoot, value); err != nil {
		fail(err)
	}
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
