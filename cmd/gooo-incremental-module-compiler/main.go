package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kimjooyoon/gooo-incremental-module-compiler/internal/compiler"
)

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: gooo-incremental-module-compiler <compile|conformance> [flags]")
	}
	switch os.Args[1] {
	case "compile":
		compileCommand(os.Args[2:])
	case "conformance":
		conformanceCommand(os.Args[2:])
	default:
		fatalf("unknown command %q", os.Args[1])
	}
}

func compileCommand(args []string) {
	flags := flag.NewFlagSet("compile", flag.ExitOnError)
	policyPath := flags.String("policy", ".gooo/incremental-compiler.gooo", "authoritative .gooo compiler policy")
	inputDir := flags.String("input-dir", "", "directory containing module .gooo files")
	scenarioPath := flags.String("scenario", "", "scenario JSON")
	modulesValue := flags.String("modules", "", "comma-separated module .gooo paths")
	outputDir := flags.String("output", "", "caller-owned output directory")
	lockPath := flags.String("upstream-lock", "contracts/upstream-lock-v1.json", "immutable optional-input lock")
	linkedGraphPath := flags.String("link-graph", "", "optional released linker graph JSON")
	runnerReleasePath := flags.String("runner-release", "", "optional immutable causal runner release JSON")
	flags.Parse(args)
	if *scenarioPath == "" || *outputDir == "" {
		fatalf("compile requires -scenario and -output")
	}
	policy, err := compiler.ParsePolicyFile(*policyPath)
	if err != nil {
		fatalf("parse policy: %v", err)
	}
	modules, err := modulePaths(*inputDir, *modulesValue)
	if err != nil {
		fatalf("resolve modules: %v", err)
	}
	parsedModules, err := compiler.ParseModules(modules)
	if err != nil {
		fatalf("parse modules: %v", err)
	}
	var input compiler.ScenarioInput
	if err := compiler.LoadJSON(*scenarioPath, &input); err != nil {
		fatalf("load scenario: %v", err)
	}
	report, generated, err := compiler.Evaluate(parsedModules, input, policy)
	if err != nil {
		fatalf("evaluate scenario: %v", err)
	}
	report.Upstream, err = compiler.ValidateOptionalInputs(*lockPath, *linkedGraphPath, *runnerReleasePath)
	if err != nil {
		fatalf("validate optional inputs: %v", err)
	}
	if err := writeScenarioOutput(*outputDir, input.ScenarioID, report, generated); err != nil {
		fatalf("write output: %v", err)
	}
	if report.Decision == compiler.Refuted {
		os.Exit(2)
	}
}

func conformanceCommand(args []string) {
	flags := flag.NewFlagSet("conformance", flag.ExitOnError)
	policyPath := flags.String("policy", ".gooo/incremental-compiler.gooo", "authoritative .gooo compiler policy")
	manifestPath := flags.String("manifest", ".gooo/conformance.gooo", "authoritative .gooo corpus")
	rootPath := flags.String("root", ".", "repository root containing the manifest inputs")
	outputDir := flags.String("output", "", "caller-owned output directory")
	lockPath := flags.String("upstream-lock", "contracts/upstream-lock-v1.json", "immutable optional-input lock")
	linkedGraphPath := flags.String("link-graph", "", "optional released linker graph JSON")
	runnerReleasePath := flags.String("runner-release", "", "optional immutable causal runner release JSON")
	flags.Parse(args)
	if *outputDir == "" {
		fatalf("conformance requires -output")
	}
	policy, err := compiler.ParsePolicyFile(filepath.Join(*rootPath, *policyPath))
	if err != nil {
		fatalf("parse policy: %v", err)
	}
	manifest, err := compiler.ParseConformanceFile(filepath.Join(*rootPath, *manifestPath))
	if err != nil {
		fatalf("parse conformance: %v", err)
	}
	upstream, err := compiler.ValidateOptionalInputs(filepath.Join(*rootPath, *lockPath), *linkedGraphPath, *runnerReleasePath)
	if err != nil {
		fatalf("validate optional inputs: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(*outputDir, "generated"), 0o755); err != nil {
		fatalf("create output: %v", err)
	}
	corpus := compiler.ConformanceReport{Schema: "gooo/incremental-module-compiler/conformance/v1", Decision: compiler.Closed}
	for _, item := range manifest.Scenarios {
		if !item.Selected {
			continue
		}
		inputDir := filepath.Join(*rootPath, item.Input)
		modules, err := modulePaths(inputDir, "")
		if err != nil {
			fatalf("resolve %s modules: %v", item.Name, err)
		}
		parsedModules, err := compiler.ParseModules(modules)
		if err != nil {
			fatalf("parse %s modules: %v", item.Name, err)
		}
		var input compiler.ScenarioInput
		if err := compiler.LoadJSON(filepath.Join(inputDir, "scenario.json"), &input); err != nil {
			fatalf("load %s scenario: %v", item.Name, err)
		}
		if input.ScenarioID != item.Name {
			fatalf("scenario identity mismatch: manifest=%s input=%s", item.Name, input.ScenarioID)
		}
		report, generated, err := compiler.Evaluate(parsedModules, input, policy)
		if err != nil {
			fatalf("evaluate %s: %v", item.Name, err)
		}
		report.Upstream = upstream
		corpus.Cases = append(corpus.Cases, report)
		if report.Decision != item.Expected {
			corpus.Decision = compiler.Refuted
		}
		if err := writeScenarioOutput(*outputDir, item.Name, report, generated); err != nil {
			fatalf("write %s output: %v", item.Name, err)
		}
	}
	if len(corpus.Cases) != len(selectedScenarios(manifest)) {
		fatalf("selected corpus count changed")
	}
	for _, report := range corpus.Cases {
		corpus.Summary.TotalCases++
		corpus.Summary.GeneratedBytes += report.Warm.GeneratedBytes
		corpus.Summary.TotalTests += report.Warm.Counts.Total
		corpus.Summary.SelectedTests += report.Warm.Counts.Selected
		corpus.Summary.ExecutedTests += report.Warm.Counts.Executed
		corpus.Summary.ReusedTests += report.Warm.Counts.Reused
		corpus.Summary.FailedTests += report.Warm.Counts.Failed
		corpus.Summary.UnknownTests += report.Warm.Counts.Unknown
		switch report.Decision {
		case compiler.Closed:
			corpus.Summary.Closed++
		case compiler.Unknown:
			corpus.Summary.Unknown++
		case compiler.Refuted:
			corpus.Summary.Refuted++
		}
	}
	if err := compiler.WriteJSON(filepath.Join(*outputDir, "conformance.json"), corpus); err != nil {
		fatalf("write conformance report: %v", err)
	}
	if corpus.Decision == compiler.Refuted {
		os.Exit(2)
	}
}

func modulePaths(inputDir, modulesValue string) ([]string, error) {
	if modulesValue != "" {
		parts := strings.Split(modulesValue, ",")
		paths := make([]string, 0, len(parts))
		for _, part := range parts {
			if strings.TrimSpace(part) != "" {
				paths = append(paths, strings.TrimSpace(part))
			}
		}
		sort.Strings(paths)
		return paths, nil
	}
	if inputDir == "" {
		return nil, fmt.Errorf("module input directory or module paths are required")
	}
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return nil, err
	}
	paths := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".gooo") {
			paths = append(paths, filepath.Join(inputDir, entry.Name()))
		}
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no module .gooo files in %s", inputDir)
	}
	return paths, nil
}

func writeScenarioOutput(outputDir, scenarioID string, report compiler.Report, generated []byte) error {
	if err := os.MkdirAll(filepath.Join(outputDir, "generated"), 0o755); err != nil {
		return err
	}
	if err := compiler.WriteJSON(filepath.Join(outputDir, scenarioID+".json"), report); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outputDir, "generated", scenarioID+".go"), generated, 0o644)
}

func selectedScenarios(manifest compiler.ConformanceManifest) []compiler.ManifestScenario {
	selected := []compiler.ManifestScenario{}
	for _, scenario := range manifest.Scenarios {
		if scenario.Selected {
			selected = append(selected, scenario)
		}
	}
	return selected
}

func fatalf(format string, values ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", values...)
	os.Exit(1)
}
