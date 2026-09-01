package compiler

import (
	"fmt"
	"regexp"
	"sort"
)

var (
	identityPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*$`)
	releasePattern  = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
	symbolPattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func validatePolicy(policy Policy) error {
	if policy.Schema != "gooo-incremental-module-compiler/v1" {
		return fmt.Errorf("unsupported policy schema %q", policy.Schema)
	}
	if policy.Denominator.ID == "" || policy.Denominator.Count != 12 || len(policy.Cells) != 12 {
		return fmt.Errorf("policy denominator must contain exactly twelve cells")
	}
	wantPrecedence := []Decision{Refuted, Unknown, Closed}
	if len(policy.Precedence) != len(wantPrecedence) {
		return fmt.Errorf("policy precedence must be REFUTED, UNKNOWN, CLOSED")
	}
	for index := range wantPrecedence {
		if policy.Precedence[index] != wantPrecedence[index] {
			return fmt.Errorf("policy precedence must be REFUTED, UNKNOWN, CLOSED")
		}
	}
	wantUnknownFields := []string{"stage", "step", "reason", "unknown_class", "next_operation", "blocked_by"}
	if len(policy.UnknownFields) != len(wantUnknownFields) {
		return fmt.Errorf("policy UNKNOWN tuple has the wrong field count")
	}
	for index := range wantUnknownFields {
		if policy.UnknownFields[index] != wantUnknownFields[index] {
			return fmt.Errorf("policy UNKNOWN tuple field %d is %q, want %q", index, policy.UnknownFields[index], wantUnknownFields[index])
		}
	}
	seen := map[string]bool{}
	for _, cell := range policy.Cells {
		if cell.ID == "" || cell.Stage == "" || cell.Step == "" || cell.Closed == "" || cell.Unknown == "" || cell.Refuted == "" || seen[cell.ID] {
			return fmt.Errorf("policy cell is missing a field or is duplicated")
		}
		seen[cell.ID] = true
	}
	if policy.Generation.Language != "go" || policy.Generation.Package == "" || policy.Generation.Entrypoint == "" {
		return fmt.Errorf("policy must declare a Go generation plan")
	}
	return nil
}

func validateModuleDeclaration(module Module) error {
	if !identityPattern.MatchString(module.Identity) {
		return fmt.Errorf("invalid module identity %q", module.Identity)
	}
	if !releasePattern.MatchString(module.Release) {
		return fmt.Errorf("module %q has an invalid release", module.Identity)
	}
	if err := ValidateDigest("module semantic_digest", module.SemanticDigest, false); err != nil {
		return err
	}
	seenExports := map[string]bool{}
	for _, symbol := range module.Exports {
		if !symbolPattern.MatchString(symbol) || seenExports[symbol] {
			return fmt.Errorf("module %q has a duplicate or invalid export", module.Identity)
		}
		seenExports[symbol] = true
	}
	seenInvalidations := map[string]bool{}
	for _, rule := range module.Invalidations {
		if !identityPattern.MatchString(rule.ID) || rule.Kind == "" || rule.Scope == "" || seenInvalidations[rule.ID] {
			return fmt.Errorf("module %q has a malformed invalidation rule", module.Identity)
		}
		seenInvalidations[rule.ID] = true
	}
	seenTests := map[string]bool{}
	for _, test := range module.Tests {
		if !identityPattern.MatchString(test.TestID) || test.Owner != module.Identity || len(test.Covers) == 0 || test.Kind == "" || len(test.Command) == 0 || seenTests[test.TestID] {
			return fmt.Errorf("module %q has malformed test ownership", module.Identity)
		}
		seenTests[test.TestID] = true
	}
	seenEdges := map[string]bool{}
	for _, dependency := range module.Dependencies {
		if !identityPattern.MatchString(dependency.Target) || !releasePattern.MatchString(dependency.Release) || dependency.EdgeID == "" || dependency.Invalidates == "" || seenEdges[dependency.EdgeID] {
			return fmt.Errorf("module %q has a malformed dependency edge", module.Identity)
		}
		if err := ValidateDigest("dependency semantic_digest", dependency.SemanticDigest, true); err != nil {
			return err
		}
		seenEdges[dependency.EdgeID] = true
	}
	return nil
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
