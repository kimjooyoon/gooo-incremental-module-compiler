package compiler

import (
	"fmt"
	"sort"
	"strings"
)

type evaluationState struct {
	Unknowns    []UnknownEvidence
	Refutations []string
}

func Evaluate(modules []Module, input ScenarioInput, policy Policy) (Report, []byte, error) {
	if err := validatePolicy(policy); err != nil {
		return Report{}, nil, err
	}
	if err := validateScenarioInput(input); err != nil {
		return Report{}, nil, err
	}
	orderedModules := append([]Module(nil), modules...)
	sort.Slice(orderedModules, func(i, j int) bool { return orderedModules[i].Identity < orderedModules[j].Identity })
	state := evaluationState{}
	graph := buildGraph(orderedModules, &state)
	scenarioDigest, err := ScenarioDigest(input)
	if err != nil {
		return Report{}, nil, err
	}
	if input.SourceDigest == "" {
		state.Unknowns = append(state.Unknowns, unknownEvidence("METRICS", "bind-source-digest", "SOURCE_DIGEST_UNAVAILABLE", "DIRECT_MISSING", "PROVIDE_SOURCE_DIGEST", "source_digest"))
	}
	if input.ToolchainDigest == "" || input.ContractDigest == "" {
		state.Unknowns = append(state.Unknowns, unknownEvidence("METRICS", "bind-comparison-digests", "COMPARISON_DIGEST_UNAVAILABLE", "DIRECT_MISSING", "PROVIDE_TOOLCHAIN_AND_CONTRACT_DIGESTS", "comparison-digests"))
	}
	affected := computeAffected(orderedModules, input.Changes, &state)
	generated, err := EmitGo(graph, policy.Generation)
	if err != nil {
		return Report{}, nil, err
	}
	graphDigest := graph.CanonicalDigest
	if graphDigest == "" {
		return Report{}, nil, fmt.Errorf("graph canonical digest is empty")
	}
	// Cold and warm runs share the same graph and scenario identity. Cache entries are
	// only consulted by the warm run; the cold run is the paired baseline.
	cold := runMode("COLD", orderedModules, input, graph, moduleIDs(orderedModules), policy, &state)
	warm := runMode("WARM", orderedModules, input, graph, affected, policy, &state)
	checkOracle(input, cold, &state)
	checkOracle(input, warm, &state)
	decision, reason := aggregateDecision(state.Refutations, state.Unknowns)
	cold.Decision = decisionForRun(cold, state)
	warm.Decision = decisionForRun(warm, state)
	generatedDigest := DigestBytes(generated)
	bytes := int64(len(generated))
	cold.GeneratedBytes = bytes
	warm.GeneratedBytes = bytes
	cold.Measurement.GeneratedBytes = bytes
	warm.Measurement.GeneratedBytes = bytes
	performance := buildImprovement(input, scenarioDigest, graphDigest, decision)
	return Report{
		Schema: ReportSchema, Protocol: ProtocolSchema, ScenarioID: input.ScenarioID,
		Decision: decision, Reason: reason,
		Digests: DigestBindings{Source: input.SourceDigest, Scenario: scenarioDigest, Contract: input.ContractDigest, Toolchain: input.ToolchainDigest, Graph: graphDigest},
		Graph:   graph, Cold: cold, Warm: warm,
		Metrics:     MetricPair{Cold: cold.Measurement, Warm: warm.Measurement},
		Improvement: performance,
		Unknowns:    uniqueUnknowns(state.Unknowns), Refutations: uniqueStrings(state.Refutations),
		GeneratedArtifacts: []ArtifactRecord{{Name: "generated/" + input.ScenarioID + ".go", Bytes: bytes, Digest: generatedDigest}},
	}, generated, nil
}

func buildGraph(modules []Module, state *evaluationState) Graph {
	graph := Graph{Schema: GraphSchema, Status: Closed, Modules: append([]Module(nil), modules...)}
	byID := map[string]Module{}
	for _, module := range modules {
		if _, exists := byID[module.Identity]; exists {
			state.Refutations = append(state.Refutations, "DUPLICATE_MODULE_"+module.Identity)
			continue
		}
		byID[module.Identity] = module
		computed, err := ModuleSemanticDigest(module)
		if err != nil {
			state.Refutations = append(state.Refutations, "SEMANTIC_DIGEST_COMPUTATION_FAILED_"+module.Identity)
		} else if computed != module.SemanticDigest {
			state.Refutations = append(state.Refutations, "SEMANTIC_DIGEST_CONTRADICTED_"+module.Identity)
		}
	}
	exportOwners := map[string]string{}
	for _, module := range modules {
		for _, symbol := range module.Exports {
			if prior, exists := exportOwners[symbol]; exists && prior != module.Identity {
				state.Refutations = append(state.Refutations, "DUPLICATE_EXPORT_"+symbol)
			} else {
				exportOwners[symbol] = module.Identity
			}
		}
	}
	seenTests := map[string]string{}
	for _, module := range modules {
		for _, test := range module.Tests {
			if prior, exists := seenTests[test.TestID]; exists && prior != module.Identity {
				state.Refutations = append(state.Refutations, "DUPLICATE_TEST_"+test.TestID)
			}
			seenTests[test.TestID] = module.Identity
			for _, covered := range test.Covers {
				if _, exists := byID[covered]; !exists {
					state.Refutations = append(state.Refutations, "TEST_OWNERSHIP_TARGET_UNAVAILABLE_"+test.TestID)
				}
			}
		}
		for _, dependency := range module.Dependencies {
			target, exists := byID[dependency.Target]
			edge := GraphEdge{EdgeID: dependency.EdgeID, From: module.Identity, To: dependency.Target, DeclaredDigest: dependency.SemanticDigest, Status: Closed}
			if !exists {
				edge.Status = Unknown
				state.Unknowns = append(state.Unknowns, unknownEvidence("GRAPH", "resolve-dependency-edge", "DEPENDENCY_MODULE_UNAVAILABLE", "DIRECT_MISSING", "PROVIDE_IMMUTABLE_DEPENDENCY_RELEASE", module.Identity+" -> "+dependency.Target))
				graph.Edges = append(graph.Edges, edge)
				continue
			}
			edge.TargetDigest = target.SemanticDigest
			if dependency.SemanticDigest == "" {
				edge.Status = Unknown
				state.Unknowns = append(state.Unknowns, unknownEvidence("GRAPH", "resolve-dependency-edge", "DEPENDENCY_SEMANTIC_DIGEST_MISSING", "DIRECT_MISSING", "PROVIDE_EXACT_DEPENDENCY_SEMANTIC_DIGEST", module.Identity+" -> "+dependency.Target))
			} else if dependency.SemanticDigest != target.SemanticDigest {
				state.Refutations = append(state.Refutations, "DEPENDENCY_SEMANTIC_DIGEST_MISMATCH_"+dependency.EdgeID)
				edge.Status = Refuted
			}
			if dependency.Release != target.Release {
				state.Refutations = append(state.Refutations, "DEPENDENCY_RELEASE_MISMATCH_"+dependency.EdgeID)
				edge.Status = Refuted
			}
			if !hasInvalidationRule(target, dependency.Invalidates) {
				state.Refutations = append(state.Refutations, "INVALIDATION_RULE_CONTRADICTED_"+dependency.EdgeID)
				edge.Status = Refuted
			}
			graph.Edges = append(graph.Edges, edge)
		}
	}
	sort.Slice(graph.Edges, func(i, j int) bool { return graphEdgeKey(graph.Edges[i]) < graphEdgeKey(graph.Edges[j]) })
	graph.Status, _ = aggregateDecision(state.Refutations, state.Unknowns)
	digest, err := GraphDigest(graph.Modules, graph.Edges)
	if err == nil {
		graph.CanonicalDigest = digest
	} else {
		state.Refutations = append(state.Refutations, "GRAPH_DIGEST_COMPUTATION_FAILED")
	}
	return graph
}

func computeAffected(modules []Module, changes []Change, state *evaluationState) []string {
	byID := map[string]Module{}
	for _, module := range modules {
		byID[module.Identity] = module
	}
	reverse := map[string][]struct {
		module string
		kind   string
	}{}
	for _, module := range modules {
		for _, dependency := range module.Dependencies {
			target, exists := byID[dependency.Target]
			if !exists {
				continue
			}
			kind := ""
			for _, rule := range target.Invalidations {
				if rule.ID == dependency.Invalidates {
					kind = rule.Kind
					break
				}
			}
			reverse[dependency.Target] = append(reverse[dependency.Target], struct {
				module string
				kind   string
			}{module: module.Identity, kind: kind})
		}
	}
	seen := map[string]bool{}
	queue := []string{}
	for _, change := range changes {
		if _, exists := byID[change.Module]; !exists {
			state.Unknowns = append(state.Unknowns, unknownEvidence("INVALIDATION", "resolve-change-module", "CHANGED_MODULE_UNAVAILABLE", "DIRECT_MISSING", "PROVIDE_CHANGED_MODULE_DECLARATION", change.Module))
			continue
		}
		switch change.Kind {
		case "semantic":
			if !seen[change.Module] {
				seen[change.Module] = true
				queue = append(queue, change.Module)
			}
		case "nonsemantic":
			// The declaration deliberately makes comments/nonsemantic edits reusable.
		default:
			state.Unknowns = append(state.Unknowns, unknownEvidence("INVALIDATION", "classify-change-kind", "CHANGE_KIND_UNSUPPORTED", "UNSUPPORTED_INPUT", "DECLARE_SEMANTIC_OR_NONSEMANTIC_CHANGE", change.Module))
		}
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, dependent := range reverse[current] {
			if dependent.kind != "semantic" || seen[dependent.module] {
				continue
			}
			seen[dependent.module] = true
			queue = append(queue, dependent.module)
		}
	}
	result := make([]string, 0, len(seen))
	for module := range seen {
		result = append(result, module)
	}
	sort.Strings(result)
	return result
}

func moduleIDs(modules []Module) []string {
	result := make([]string, 0, len(modules))
	for _, module := range modules {
		result = append(result, module.Identity)
	}
	return result
}

func runMode(mode string, modules []Module, input ScenarioInput, graph Graph, affected []string, policy Policy, state *evaluationState) RunReport {
	affectedSet := map[string]bool{}
	for _, module := range affected {
		affectedSet[module] = true
	}
	cacheByModule := map[string]CacheEntry{}
	for _, entry := range input.Cache {
		if _, exists := cacheByModule[entry.Module]; exists {
			state.Refutations = append(state.Refutations, "DUPLICATE_CACHE_ENTRY_"+entry.Module)
			continue
		}
		cacheByModule[entry.Module] = entry
	}
	compiled := []string{}
	reused := []string{}
	modulePlan := []ModuleDecision{}
	for _, module := range modules {
		if mode == "COLD" || affectedSet[module.Identity] {
			compiled = append(compiled, module.Identity)
			reason := "SEMANTIC_INVALIDATION_REQUIRES_COMPILE"
			if mode == "COLD" {
				reason = "COLD_BASELINE_COMPILE"
			}
			modulePlan = append(modulePlan, ModuleDecision{Module: module.Identity, Action: "COMPILE", Reason: reason})
			continue
		}
		entry, exists := cacheByModule[module.Identity]
		if !exists {
			compiled = append(compiled, module.Identity)
			modulePlan = append(modulePlan, ModuleDecision{Module: module.Identity, Action: "COMPILE", Reason: "CACHE_MISS"})
			continue
		}
		dependencyDigest, err := DependencyDigest(module)
		exact := err == nil && entry.Immutable && entry.SemanticDigest == module.SemanticDigest && entry.DependencyDigest == dependencyDigest && entry.ToolchainDigest == input.ToolchainDigest && entry.ContractDigest == input.ContractDigest && entry.ResultDigest != ""
		if exact {
			reused = append(reused, module.Identity)
			modulePlan = append(modulePlan, ModuleDecision{Module: module.Identity, Action: "REUSE", Reason: "EXACT_SEMANTIC_CACHE_IDENTITY"})
		} else {
			state.Refutations = append(state.Refutations, "STALE_CACHE_IDENTITY_"+module.Identity)
			compiled = append(compiled, module.Identity)
			modulePlan = append(modulePlan, ModuleDecision{Module: module.Identity, Action: "COMPILE", Reason: "STALE_CACHE_IDENTITY_REFUTED"})
		}
	}
	sort.Strings(compiled)
	sort.Strings(reused)
	sort.Slice(modulePlan, func(i, j int) bool { return modulePlan[i].Module < modulePlan[j].Module })

	testCacheByID := map[string]TestCacheEntry{}
	for _, entry := range input.TestCache {
		if _, exists := testCacheByID[entry.TestID]; exists {
			state.Refutations = append(state.Refutations, "DUPLICATE_TEST_CACHE_ENTRY_"+entry.TestID)
			continue
		}
		testCacheByID[entry.TestID] = entry
	}
	observations := map[string]TestObservation{}
	for _, observation := range input.Observations {
		if _, exists := observations[observation.TestID]; exists {
			state.Refutations = append(state.Refutations, "DUPLICATE_TEST_OBSERVATION_"+observation.TestID)
			continue
		}
		observations[observation.TestID] = observation
	}
	selected, executed, reusedTests, failed, unknownTests := []string{}, []string{}, []string{}, []string{}, []string{}
	testPlan := []TestDecision{}
	for _, test := range allTests(modules) {
		impacted := mode == "COLD" || testTouches(test, affectedSet)
		entry, hasCache := testCacheByID[test.TestID]
		canReuse := mode == "WARM" && !impacted && hasCache && entry.Immutable && entry.ResultDigest != "" && entry.ToolchainDigest == input.ToolchainDigest && entry.ContractDigest == input.ContractDigest
		if canReuse {
			if observation, observed := observations[test.TestID]; observed && observation.ResultDigest != entry.ResultDigest {
				state.Refutations = append(state.Refutations, "STALE_TEST_CACHE_RESULT_"+test.TestID)
				canReuse = false
			}
		}
		if canReuse {
			reusedTests = append(reusedTests, test.TestID)
			testPlan = append(testPlan, TestDecision{TestID: test.TestID, Owner: test.Owner, Action: "REUSE", Reason: "EXACT_TEST_CACHE_IDENTITY"})
		} else {
			if mode == "WARM" && !impacted && hasCache {
				state.Refutations = append(state.Refutations, "STALE_TEST_CACHE_IDENTITY_"+test.TestID)
			}
			selected = append(selected, test.TestID)
			testPlan = append(testPlan, TestDecision{TestID: test.TestID, Owner: test.Owner, Action: "EXECUTE", Reason: testReason(mode, impacted, hasCache)})
		}
		observation, observed := observations[test.TestID]
		if !observed {
			unknownTests = append(unknownTests, test.TestID)
			state.Unknowns = append(state.Unknowns, unknownEvidence("TEST_SELECTION", "bind-test-observation", "TEST_OBSERVATION_UNAVAILABLE", "DIRECT_MISSING", "PROVIDE_CURRENT_TEST_OBSERVATION", test.TestID))
			continue
		}
		if observation.Status == "FAIL" {
			failed = append(failed, test.TestID)
			state.Refutations = append(state.Refutations, "TEST_FAILED_"+test.TestID)
		} else if observation.Status != "PASS" || observation.ResultDigest == "" {
			state.Unknowns = append(state.Unknowns, unknownEvidence("TEST_SELECTION", "bind-test-observation", "TEST_OBSERVATION_UNTYPED", "UNSUPPORTED_INPUT", "PROVIDE_PASS_OR_FAIL_RESULT_DIGEST", test.TestID))
		}
		if decisionForTest(test.TestID, testPlan).Action == "EXECUTE" {
			executed = append(executed, test.TestID)
		}
	}
	sort.Strings(selected)
	sort.Strings(executed)
	sort.Strings(reusedTests)
	sort.Strings(failed)
	sort.Strings(unknownTests)
	sort.Slice(testPlan, func(i, j int) bool { return testPlan[i].TestID < testPlan[j].TestID })
	return RunReport{
		Mode: mode, CompiledModules: compiled, ReusedModules: reused,
		ModulePlan: modulePlan, SelectedTests: selected, ExecutedTests: executed,
		ReusedTests: reusedTests, FailedTests: failed, UnknownTests: unknownTests,
		TestPlan:    testPlan,
		Counts:      TestCounts{Total: len(testPlan), Selected: len(selected), Executed: len(executed), Reused: len(reusedTests), Failed: len(failed), Unknown: len(unknownTests)},
		Measurement: measurementFor(mode, input.Metrics),
	}
}

func checkOracle(input ScenarioInput, run RunReport, state *evaluationState) {
	if !input.FullOracle.Independent || input.FullOracle.Digest == "" {
		state.Unknowns = append(state.Unknowns, unknownEvidence("ORACLE", "bind-independent-full-oracle", "FULL_ORACLE_UNAVAILABLE", "DIRECT_MISSING", "PROVIDE_INDEPENDENT_FULL_ORACLE", run.Mode))
		return
	}
	actualDigest, err := DigestJSON(input.FullOracle.Results)
	if err != nil || actualDigest != input.FullOracle.Digest {
		state.Refutations = append(state.Refutations, "FULL_ORACLE_DIGEST_CONTRADICTED_"+run.Mode)
		return
	}
	oracleByID := map[string]TestObservation{}
	for _, result := range input.FullOracle.Results {
		if _, exists := oracleByID[result.TestID]; exists {
			state.Refutations = append(state.Refutations, "DUPLICATE_FULL_ORACLE_RESULT_"+result.TestID)
			continue
		}
		oracleByID[result.TestID] = result
	}
	for _, testID := range append(append([]string{}, run.SelectedTests...), run.ReusedTests...) {
		observed, exists := findObservation(input.Observations, testID)
		oracle, oracleExists := oracleByID[testID]
		if !exists || !oracleExists {
			state.Unknowns = append(state.Unknowns, unknownEvidence("ORACLE", "compare-selective-with-full", "ORACLE_TEST_RESULT_UNAVAILABLE", "DIRECT_MISSING", "PROVIDE_COMPLETE_FULL_ORACLE_RESULT", testID))
			continue
		}
		if observed.Status != oracle.Status || observed.ResultDigest != oracle.ResultDigest {
			state.Refutations = append(state.Refutations, "SELECTIVE_FULL_ORACLE_MISMATCH_"+testID)
		}
	}
}

func buildImprovement(input ScenarioInput, scenarioDigest, graphDigest string, decision Decision) ImprovementClaim {
	claim := ImprovementClaim{Status: Unknown, Before: input.Metrics.Cold, After: input.Metrics.Warm}
	claim.MatchedDigests = validComparisonDigest(input.SourceDigest) && validComparisonDigest(input.ToolchainDigest) && validComparisonDigest(input.ContractDigest) && validComparisonDigest(scenarioDigest) && validComparisonDigest(graphDigest)
	if !claim.MatchedDigests {
		claim.Reason = "IMPROVEMENT_UNKNOWN_UNMATCHED_DIGESTS"
		return claim
	}
	if decision != Closed {
		claim.Reason = "IMPROVEMENT_UNKNOWN_SCENARIO_NOT_CLOSED"
		return claim
	}
	if claim.Before.TotalMS <= claim.After.TotalMS {
		claim.Reason = "IMPROVEMENT_UNKNOWN_WARM_RUN_NOT_STRICTLY_FASTER"
		return claim
	}
	delta := claim.Before.TotalMS - claim.After.TotalMS
	claim.Status = Closed
	claim.Reason = "MATCHED_EXACT_PAIR"
	claim.ImprovementMS = &delta
	return claim
}

func validateScenarioInput(input ScenarioInput) error {
	if input.Schema != "gooo/incremental-module-compiler/scenario/v1" || input.ScenarioID == "" {
		return fmt.Errorf("malformed scenario input")
	}
	if err := ValidateDigest("source_digest", input.SourceDigest, true); err != nil {
		return err
	}
	if err := ValidateDigest("toolchain_digest", input.ToolchainDigest, true); err != nil {
		return err
	}
	if err := ValidateDigest("contract_digest", input.ContractDigest, true); err != nil {
		return err
	}
	for _, change := range input.Changes {
		if change.Module == "" || change.Kind == "" {
			return fmt.Errorf("scenario change is incomplete")
		}
	}
	for _, observation := range input.Observations {
		if observation.TestID == "" || (observation.Status != "PASS" && observation.Status != "FAIL") || observation.ResultDigest == "" {
			return fmt.Errorf("scenario observation is malformed")
		}
	}
	for _, value := range []int64{
		input.Metrics.Cold.CompileMS, input.Metrics.Cold.BuildMS, input.Metrics.Cold.TestMS, input.Metrics.Cold.ConformanceMS, input.Metrics.Cold.IntegrationMS, input.Metrics.Cold.TotalMS, input.Metrics.Cold.PeakRSSKiB,
		input.Metrics.Warm.CompileMS, input.Metrics.Warm.BuildMS, input.Metrics.Warm.TestMS, input.Metrics.Warm.ConformanceMS, input.Metrics.Warm.IntegrationMS, input.Metrics.Warm.TotalMS, input.Metrics.Warm.PeakRSSKiB,
	} {
		if value < 0 {
			return fmt.Errorf("scenario measurements must be nonnegative exact integers")
		}
	}
	return nil
}

func hasInvalidationRule(module Module, ruleID string) bool {
	for _, rule := range module.Invalidations {
		if rule.ID == ruleID {
			return true
		}
	}
	return false
}

func allTests(modules []Module) []TestOwnership {
	var tests []TestOwnership
	for _, module := range modules {
		tests = append(tests, module.Tests...)
	}
	sort.Slice(tests, func(i, j int) bool { return tests[i].TestID < tests[j].TestID })
	return tests
}

func testTouches(test TestOwnership, affected map[string]bool) bool {
	if affected[test.Owner] {
		return true
	}
	for _, module := range test.Covers {
		if affected[module] {
			return true
		}
	}
	return false
}

func testReason(mode string, impacted, hasCache bool) string {
	if mode == "COLD" {
		return "COLD_BASELINE_TEST_EXECUTION"
	}
	if impacted {
		return "SEMANTIC_INVALIDATION_REQUIRES_TEST_EXECUTION"
	}
	if hasCache {
		return "TEST_CACHE_IDENTITY_REFUTED"
	}
	return "TEST_CACHE_MISS"
}

func decisionForTest(testID string, plans []TestDecision) TestDecision {
	for _, plan := range plans {
		if plan.TestID == testID {
			return plan
		}
	}
	return TestDecision{}
}

func findObservation(observations []TestObservation, testID string) (TestObservation, bool) {
	for _, observation := range observations {
		if observation.TestID == testID {
			return observation, true
		}
	}
	return TestObservation{}, false
}

func measurementFor(mode string, pair MetricPair) Measurement {
	if mode == "COLD" {
		return pair.Cold
	}
	return pair.Warm
}

func decisionForRun(run RunReport, state evaluationState) Decision {
	if len(state.Refutations) > 0 {
		return Refuted
	}
	if len(state.Unknowns) > 0 {
		return Unknown
	}
	return Closed
}

func aggregateDecision(refutations []string, unknowns []UnknownEvidence) (Decision, string) {
	if len(refutations) > 0 {
		return Refuted, uniqueStrings(refutations)[0]
	}
	if len(unknowns) > 0 {
		return Unknown, uniqueUnknowns(unknowns)[0].Reason
	}
	return Closed, "EXACT_INCREMENTAL_COMPILATION_CLOSED"
}

func unknownEvidence(stage, step, reason, class, next, blocked string) UnknownEvidence {
	return UnknownEvidence{Stage: stage, Step: step, Reason: reason, UnknownClass: class, NextOperation: next, BlockedBy: blocked}
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func uniqueUnknowns(values []UnknownEvidence) []UnknownEvidence {
	seen := map[string]bool{}
	result := []UnknownEvidence{}
	for _, value := range values {
		key := strings.Join([]string{value.Stage, value.Step, value.Reason, value.UnknownClass, value.NextOperation, value.BlockedBy}, "\x00")
		if !seen[key] {
			seen[key] = true
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left := result[i].Stage + "\x00" + result[i].Step + "\x00" + result[i].Reason + "\x00" + result[i].BlockedBy
		right := result[j].Stage + "\x00" + result[j].Step + "\x00" + result[j].Reason + "\x00" + result[j].BlockedBy
		return left < right
	})
	return result
}

func validComparisonDigest(value string) bool {
	return ValidateDigest("comparison", value, false) == nil
}
