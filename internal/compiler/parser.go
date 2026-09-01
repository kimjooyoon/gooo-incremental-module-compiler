package compiler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func ParsePolicyFile(path string) (Policy, error) {
	var policy Policy
	inside := false
	seenHeader := false
	err := forLines(path, func(lineNo int, line string) error {
		tokens, err := tokenize(line)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		if !seenHeader {
			if len(tokens) != 3 || tokens[0] != "compiler" || tokens[2] != "{" {
				return fmt.Errorf("line %d: expected compiler header", lineNo)
			}
			policy.Schema = tokens[1]
			seenHeader, inside = true, true
			return nil
		}
		if tokens[0] == "}" {
			if len(tokens) != 1 || !inside {
				return fmt.Errorf("line %d: unexpected closing brace", lineNo)
			}
			inside = false
			return nil
		}
		if !inside {
			return fmt.Errorf("line %d: content after compiler body", lineNo)
		}
		switch tokens[0] {
		case "precedence":
			if len(tokens) != 4 {
				return fmt.Errorf("line %d: precedence requires three statuses", lineNo)
			}
			for _, value := range tokens[1:] {
				policy.Precedence = append(policy.Precedence, Decision(value))
			}
		case "unknown_fields":
			policy.UnknownFields = append([]string(nil), tokens[1:]...)
		case "denominator":
			if len(tokens) < 3 {
				return fmt.Errorf("line %d: denominator identity is required", lineNo)
			}
			pairs, err := pairsAfter(tokens, 2)
			if err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
			policy.Denominator.ID = tokens[1]
			if pairs["count"] == "" {
				return fmt.Errorf("line %d: denominator count is required", lineNo)
			}
			policy.Denominator.Count, err = strconv.Atoi(pairs["count"])
			if err != nil {
				return fmt.Errorf("line %d: malformed denominator count", lineNo)
			}
		case "cell":
			if len(tokens) < 3 {
				return fmt.Errorf("line %d: cell identity is required", lineNo)
			}
			pairs, err := pairsAfter(tokens, 2)
			if err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
			policy.Cells = append(policy.Cells, PolicyCell{ID: tokens[1], Stage: pairs["stage"], Step: pairs["step"], Closed: pairs["closed"], Unknown: pairs["unknown"], Refuted: pairs["refuted"]})
		case "generation":
			if len(tokens) < 3 {
				return fmt.Errorf("line %d: generation language is required", lineNo)
			}
			pairs, err := pairsAfter(tokens, 2)
			if err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
			policy.Generation = GenerationPlan{Language: tokens[1], Package: pairs["package"], Entrypoint: pairs["entrypoint"]}
		default:
			return fmt.Errorf("line %d: unknown compiler record %q", lineNo, tokens[0])
		}
		return nil
	})
	if err != nil {
		return Policy{}, err
	}
	if !seenHeader || inside {
		return Policy{}, fmt.Errorf("%s: incomplete compiler declaration", path)
	}
	if err := validatePolicy(policy); err != nil {
		return Policy{}, fmt.Errorf("%s: %w", path, err)
	}
	return policy, nil
}

func ParseModuleFile(path string) (Module, error) {
	var module Module
	inside := false
	seenHeader := false
	err := forLines(path, func(lineNo int, line string) error {
		tokens, err := tokenize(line)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		if !seenHeader {
			if len(tokens) != 7 || tokens[0] != "module" || tokens[2] != "release" || tokens[4] != "semantic_digest" || tokens[6] != "{" {
				return fmt.Errorf("line %d: expected module header", lineNo)
			}
			module = Module{Identity: tokens[1], Release: tokens[3], SemanticDigest: tokens[5]}
			seenHeader, inside = true, true
			return nil
		}
		if tokens[0] == "}" {
			if len(tokens) != 1 || !inside {
				return fmt.Errorf("line %d: unexpected closing brace", lineNo)
			}
			inside = false
			return nil
		}
		if !inside {
			return fmt.Errorf("line %d: content after module body", lineNo)
		}
		switch tokens[0] {
		case "export":
			if len(tokens) != 2 {
				return fmt.Errorf("line %d: export requires a symbol", lineNo)
			}
			module.Exports = append(module.Exports, tokens[1])
		case "dependency":
			if len(tokens) < 3 {
				return fmt.Errorf("line %d: dependency target is required", lineNo)
			}
			pairs, err := pairsAfter(tokens, 2)
			if err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
			module.Dependencies = append(module.Dependencies, Dependency{Target: tokens[1], Release: pairs["release"], SemanticDigest: pairs["semantic_digest"], EdgeID: pairs["edge"], Invalidates: pairs["invalidates"]})
		case "invalidation":
			if len(tokens) < 3 {
				return fmt.Errorf("line %d: invalidation identity is required", lineNo)
			}
			pairs, err := pairsAfter(tokens, 2)
			if err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
			module.Invalidations = append(module.Invalidations, InvalidationRule{ID: tokens[1], Kind: pairs["kind"], Scope: pairs["scope"]})
		case "test_owner":
			if len(tokens) < 3 {
				return fmt.Errorf("line %d: test identity is required", lineNo)
			}
			pairs, err := pairsAfter(tokens, 2)
			if err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
			command := strings.Fields(pairs["command"])
			module.Tests = append(module.Tests, TestOwnership{TestID: tokens[1], Owner: pairs["owner"], Covers: csv(pairs["covers"]), Kind: pairs["kind"], Command: command})
		default:
			return fmt.Errorf("line %d: unknown module record %q", lineNo, tokens[0])
		}
		return nil
	})
	if err != nil {
		return Module{}, err
	}
	if !seenHeader || inside {
		return Module{}, fmt.Errorf("%s: incomplete module declaration", path)
	}
	if err := validateModuleDeclaration(module); err != nil {
		return Module{}, fmt.Errorf("%s: %w", path, err)
	}
	return module, nil
}

func ParseModules(paths []string) ([]Module, error) {
	modules := make([]Module, 0, len(paths))
	for _, path := range paths {
		module, err := ParseModuleFile(path)
		if err != nil {
			return nil, err
		}
		modules = append(modules, module)
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Identity < modules[j].Identity })
	return modules, nil
}

func ParseConformanceFile(path string) (ConformanceManifest, error) {
	var manifest ConformanceManifest
	inside := false
	seenHeader := false
	err := forLines(path, func(lineNo int, line string) error {
		tokens, err := tokenize(line)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		if !seenHeader {
			if len(tokens) != 3 || tokens[0] != "conformance" || tokens[2] != "{" {
				return fmt.Errorf("line %d: expected conformance header", lineNo)
			}
			manifest.Schema = tokens[1]
			seenHeader, inside = true, true
			continue
		}
		if tokens[0] == "}" {
			inside = false
			continue
		}
		if !inside || tokens[0] != "scenario" {
			return fmt.Errorf("line %d: expected scenario record", lineNo)
		}
		if len(tokens) < 3 {
			return fmt.Errorf("line %d: scenario identity is required", lineNo)
		}
		pairs, err := pairsAfter(tokens, 2)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
		selected, err := strconv.ParseBool(pairs["selected"])
		if err != nil {
			return fmt.Errorf("line %d: malformed selected flag", lineNo)
		}
		manifest.Scenarios = append(manifest.Scenarios, ManifestScenario{Name: tokens[1], Expected: Decision(pairs["expect"]), Input: pairs["input"], Selected: selected})
		return nil
	})
	if err != nil {
		return ConformanceManifest{}, err
	}
	if !seenHeader || inside || manifest.Schema == "" || len(manifest.Scenarios) == 0 {
		return ConformanceManifest{}, fmt.Errorf("%s: incomplete or empty conformance declaration", path)
	}
	return manifest, nil
}

func LoadJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func forLines(path string, fn func(int, string) error) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := cleanLine(scanner.Text())
		if line == "" {
			continue
		}
		if err := fn(lineNo, line); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

func cleanLine(line string) string {
	line = strings.TrimSpace(line)
	if index := strings.Index(line, "#"); index >= 0 {
		line = strings.TrimSpace(line[:index])
	}
	if strings.HasPrefix(line, "//") {
		return ""
	}
	return line
}

func tokenize(line string) ([]string, error) {
	var tokens []string
	for index := 0; index < len(line); {
		for index < len(line) && (line[index] == ' ' || line[index] == '\t') {
			index++
		}
		if index == len(line) {
			break
		}
		if line[index] == '"' {
			start := index
			index++
			escaped, closed := false, false
			for index < len(line) {
				if escaped {
					escaped = false
					index++
					continue
				}
				if line[index] == '\\' {
					escaped = true
					index++
					continue
				}
				if line[index] == '"' {
					index++
					closed = true
					break
				}
				index++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated quoted value")
			}
			value, err := strconv.Unquote(line[start:index])
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, value)
			continue
		}
		start := index
		for index < len(line) && line[index] != ' ' && line[index] != '\t' {
			index++
		}
		tokens = append(tokens, line[start:index])
	}
	return tokens, nil
}

func pairsAfter(tokens []string, start int) (map[string]string, error) {
	if len(tokens[start:])%2 != 0 {
		return nil, fmt.Errorf("expected key/value pairs")
	}
	pairs := make(map[string]string, len(tokens[start:])/2)
	for index := start; index < len(tokens); index += 2 {
		key := tokens[index]
		if key == "" || key == "{" || key == "}" || pairs[key] != "" {
			return nil, fmt.Errorf("duplicate or malformed key %q", key)
		}
		pairs[key] = tokens[index+1]
	}
	return pairs, nil
}

func csv(value string) []string {
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}
