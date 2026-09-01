package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

func DigestBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func DigestFile(path string) (string, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	return DigestBytes(data), data, nil
}

func DigestJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return DigestBytes(data), nil
}

type moduleSemanticView struct {
	Identity      string             `json:"identity"`
	Release       string             `json:"release"`
	Exports       []string           `json:"exports"`
	Dependencies  []Dependency       `json:"dependencies"`
	Invalidations []InvalidationRule `json:"invalidations"`
	Tests         []TestOwnership    `json:"tests"`
}

func ModuleSemanticDigest(module Module) (string, error) {
	view := moduleSemanticView{
		Identity: module.Identity, Release: module.Release,
		Exports:       append([]string(nil), module.Exports...),
		Dependencies:  append([]Dependency(nil), module.Dependencies...),
		Invalidations: append([]InvalidationRule(nil), module.Invalidations...),
		Tests:         append([]TestOwnership(nil), module.Tests...),
	}
	if view.Exports == nil {
		view.Exports = []string{}
	}
	if view.Dependencies == nil {
		view.Dependencies = []Dependency{}
	}
	if view.Invalidations == nil {
		view.Invalidations = []InvalidationRule{}
	}
	if view.Tests == nil {
		view.Tests = []TestOwnership{}
	}
	sort.Strings(view.Exports)
	sort.Slice(view.Dependencies, func(i, j int) bool { return dependencyKey(view.Dependencies[i]) < dependencyKey(view.Dependencies[j]) })
	sort.Slice(view.Invalidations, func(i, j int) bool {
		return invalidationKey(view.Invalidations[i]) < invalidationKey(view.Invalidations[j])
	})
	sort.Slice(view.Tests, func(i, j int) bool { return view.Tests[i].TestID < view.Tests[j].TestID })
	for i := range view.Tests {
		view.Tests[i].Covers = append([]string(nil), view.Tests[i].Covers...)
		sort.Strings(view.Tests[i].Covers)
		view.Tests[i].Command = append([]string(nil), view.Tests[i].Command...)
	}
	return DigestJSON(view)
}

func DependencyDigest(module Module) (string, error) {
	dependencies := append([]Dependency(nil), module.Dependencies...)
	if dependencies == nil {
		dependencies = []Dependency{}
	}
	sort.Slice(dependencies, func(i, j int) bool { return dependencyKey(dependencies[i]) < dependencyKey(dependencies[j]) })
	return DigestJSON(dependencies)
}

func GraphDigest(modules []Module, edges []GraphEdge) (string, error) {
	orderedModules := append([]Module(nil), modules...)
	orderedEdges := append([]GraphEdge(nil), edges...)
	for index := range orderedModules {
		orderedModules[index].Exports = append([]string(nil), orderedModules[index].Exports...)
		sort.Strings(orderedModules[index].Exports)
		orderedModules[index].Dependencies = append([]Dependency(nil), orderedModules[index].Dependencies...)
		sort.Slice(orderedModules[index].Dependencies, func(i, j int) bool {
			return dependencyKey(orderedModules[index].Dependencies[i]) < dependencyKey(orderedModules[index].Dependencies[j])
		})
		orderedModules[index].Invalidations = append([]InvalidationRule(nil), orderedModules[index].Invalidations...)
		sort.Slice(orderedModules[index].Invalidations, func(i, j int) bool {
			return invalidationKey(orderedModules[index].Invalidations[i]) < invalidationKey(orderedModules[index].Invalidations[j])
		})
		orderedModules[index].Tests = append([]TestOwnership(nil), orderedModules[index].Tests...)
		sort.Slice(orderedModules[index].Tests, func(i, j int) bool {
			return orderedModules[index].Tests[i].TestID < orderedModules[index].Tests[j].TestID
		})
		for testIndex := range orderedModules[index].Tests {
			orderedModules[index].Tests[testIndex].Covers = append([]string(nil), orderedModules[index].Tests[testIndex].Covers...)
			sort.Strings(orderedModules[index].Tests[testIndex].Covers)
			orderedModules[index].Tests[testIndex].Command = append([]string(nil), orderedModules[index].Tests[testIndex].Command...)
		}
	}
	sort.Slice(orderedModules, func(i, j int) bool { return orderedModules[i].Identity < orderedModules[j].Identity })
	sort.Slice(orderedEdges, func(i, j int) bool { return graphEdgeKey(orderedEdges[i]) < graphEdgeKey(orderedEdges[j]) })
	return DigestJSON(struct {
		Schema  string      `json:"schema"`
		Modules []Module    `json:"modules"`
		Edges   []GraphEdge `json:"edges"`
	}{Schema: GraphSchema, Modules: orderedModules, Edges: orderedEdges})
}

func ScenarioDigest(input ScenarioInput) (string, error) {
	changes := append([]Change(nil), input.Changes...)
	sort.Slice(changes, func(i, j int) bool {
		left := changes[i].Module + "\x00" + changes[i].Kind + "\x00" + changes[i].Predicate
		right := changes[j].Module + "\x00" + changes[j].Kind + "\x00" + changes[j].Predicate
		return left < right
	})
	return DigestJSON(struct {
		Schema     string   `json:"schema"`
		ScenarioID string   `json:"scenario_id"`
		Changes    []Change `json:"changes"`
	}{Schema: input.Schema, ScenarioID: input.ScenarioID, Changes: changes})
}

func dependencyKey(value Dependency) string {
	return value.Target + "\x00" + value.Release + "\x00" + value.SemanticDigest + "\x00" + value.EdgeID + "\x00" + value.Invalidates
}

func invalidationKey(value InvalidationRule) string {
	return value.ID + "\x00" + value.Kind + "\x00" + value.Scope
}

func graphEdgeKey(value GraphEdge) string {
	return value.From + "\x00" + value.To + "\x00" + value.EdgeID + "\x00" + value.DeclaredDigest
}

func ValidateDigest(name, value string, allowEmpty bool) error {
	if value == "" && allowEmpty {
		return nil
	}
	if len(value) != len("sha256:")+64 || value[:len("sha256:")] != "sha256:" {
		return fmt.Errorf("%s must be a sha256 digest", name)
	}
	if _, err := hex.DecodeString(value[len("sha256:"):]); err != nil {
		return fmt.Errorf("%s must contain lowercase hexadecimal", name)
	}
	return nil
}
