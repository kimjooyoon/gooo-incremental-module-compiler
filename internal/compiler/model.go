package compiler

const (
	ProtocolSchema          = "gooo/incremental-module-compiler/protocol/v1"
	GraphSchema             = "gooo/incremental-module-compiler/graph/v1"
	ReportSchema            = "gooo/incremental-module-compiler/report/v1"
	Closed         Decision = "CLOSED"
	Unknown        Decision = "UNKNOWN"
	Refuted        Decision = "REFUTED"
)

type Decision string

type Policy struct {
	Schema        string         `json:"schema"`
	Precedence    []Decision     `json:"precedence"`
	UnknownFields []string       `json:"unknown_fields"`
	Denominator   Denominator    `json:"denominator"`
	Cells         []PolicyCell   `json:"cells"`
	Generation    GenerationPlan `json:"generation"`
}

type Denominator struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

type PolicyCell struct {
	ID      string `json:"id"`
	Stage   string `json:"stage"`
	Step    string `json:"step"`
	Closed  string `json:"closed"`
	Unknown string `json:"unknown"`
	Refuted string `json:"refuted"`
}

type GenerationPlan struct {
	Language   string `json:"language"`
	Package    string `json:"package"`
	Entrypoint string `json:"entrypoint"`
}

type Module struct {
	Identity       string             `json:"identity"`
	Release        string             `json:"release"`
	SemanticDigest string             `json:"semantic_digest"`
	Exports        []string           `json:"exports"`
	Dependencies   []Dependency       `json:"dependencies"`
	Invalidations  []InvalidationRule `json:"invalidations"`
	Tests          []TestOwnership    `json:"tests"`
}

type Dependency struct {
	Target         string `json:"target"`
	Release        string `json:"release"`
	SemanticDigest string `json:"semantic_digest"`
	EdgeID         string `json:"edge_id"`
	Invalidates    string `json:"invalidates"`
}

type InvalidationRule struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Scope string `json:"scope"`
}

type TestOwnership struct {
	TestID  string   `json:"test_id"`
	Owner   string   `json:"owner"`
	Covers  []string `json:"covers"`
	Kind    string   `json:"kind"`
	Command []string `json:"command"`
}

type Change struct {
	Module    string `json:"module"`
	Kind      string `json:"kind"`
	Predicate string `json:"predicate"`
}

type CacheEntry struct {
	Module           string `json:"module"`
	SemanticDigest   string `json:"semantic_digest"`
	DependencyDigest string `json:"dependency_digest"`
	ToolchainDigest  string `json:"toolchain_digest"`
	ContractDigest   string `json:"contract_digest"`
	ResultDigest     string `json:"result_digest"`
	Immutable        bool   `json:"immutable"`
}

type TestCacheEntry struct {
	TestID          string `json:"test_id"`
	ResultDigest    string `json:"result_digest"`
	ToolchainDigest string `json:"toolchain_digest"`
	ContractDigest  string `json:"contract_digest"`
	Immutable       bool   `json:"immutable"`
}

type TestObservation struct {
	TestID       string `json:"test_id"`
	Status       string `json:"status"`
	ResultDigest string `json:"result_digest"`
}

type FullOracle struct {
	Independent bool              `json:"independent"`
	Digest      string            `json:"digest"`
	Results     []TestObservation `json:"results"`
}

type Measurement struct {
	CompileMS      int64 `json:"compile_ms"`
	BuildMS        int64 `json:"build_ms"`
	TestMS         int64 `json:"test_ms"`
	ConformanceMS  int64 `json:"conformance_ms"`
	IntegrationMS  int64 `json:"integration_ms"`
	TotalMS        int64 `json:"total_ms"`
	PeakRSSKiB     int64 `json:"peak_rss_kib"`
	GeneratedBytes int64 `json:"generated_bytes"`
}

type MetricPair struct {
	Cold Measurement `json:"cold"`
	Warm Measurement `json:"warm"`
}

type ScenarioInput struct {
	Schema          string            `json:"schema"`
	ScenarioID      string            `json:"scenario_id"`
	SourceDigest    string            `json:"source_digest"`
	ToolchainDigest string            `json:"toolchain_digest"`
	ContractDigest  string            `json:"contract_digest"`
	Changes         []Change          `json:"changes"`
	Cache           []CacheEntry      `json:"cache"`
	TestCache       []TestCacheEntry  `json:"test_cache"`
	Observations    []TestObservation `json:"observations"`
	FullOracle      FullOracle        `json:"full_oracle"`
	Metrics         MetricPair        `json:"metrics"`
}

type UnknownEvidence struct {
	Stage         string `json:"stage"`
	Step          string `json:"step"`
	Reason        string `json:"reason"`
	UnknownClass  string `json:"unknown_class"`
	NextOperation string `json:"next_operation"`
	BlockedBy     string `json:"blocked_by"`
}

type GraphEdge struct {
	EdgeID         string   `json:"edge_id"`
	From           string   `json:"from"`
	To             string   `json:"to"`
	DeclaredDigest string   `json:"declared_digest"`
	TargetDigest   string   `json:"target_digest"`
	Status         Decision `json:"status"`
}

type Graph struct {
	Schema          string      `json:"schema"`
	Status          Decision    `json:"status"`
	Modules         []Module    `json:"modules"`
	Edges           []GraphEdge `json:"edges"`
	CanonicalDigest string      `json:"canonical_digest"`
}

type ModuleDecision struct {
	Module string `json:"module"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

type TestDecision struct {
	TestID string `json:"test_id"`
	Owner  string `json:"owner"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

type TestCounts struct {
	Total    int `json:"total"`
	Selected int `json:"selected"`
	Executed int `json:"executed"`
	Reused   int `json:"reused"`
	Failed   int `json:"failed"`
	Unknown  int `json:"unknown"`
}

type RunReport struct {
	Mode            string           `json:"mode"`
	Decision        Decision         `json:"decision"`
	CompiledModules []string         `json:"compiled_modules"`
	ReusedModules   []string         `json:"reused_modules"`
	ModulePlan      []ModuleDecision `json:"module_plan"`
	SelectedTests   []string         `json:"selected_tests"`
	ExecutedTests   []string         `json:"executed_tests"`
	ReusedTests     []string         `json:"reused_tests"`
	FailedTests     []string         `json:"failed_tests"`
	UnknownTests    []string         `json:"unknown_tests"`
	TestPlan        []TestDecision   `json:"test_plan"`
	Counts          TestCounts       `json:"tests"`
	GeneratedBytes  int64            `json:"generated_bytes"`
	Measurement     Measurement      `json:"measurement"`
}

type ImprovementClaim struct {
	Status         Decision    `json:"status"`
	Reason         string      `json:"reason"`
	MatchedDigests bool        `json:"matched_digests"`
	Before         Measurement `json:"before"`
	After          Measurement `json:"after"`
	ImprovementMS  *int64      `json:"improvement_ms,omitempty"`
}

type DigestBindings struct {
	Source    string `json:"source"`
	Scenario  string `json:"scenario"`
	Contract  string `json:"contract"`
	Toolchain string `json:"toolchain"`
	Graph     string `json:"graph"`
}

type Report struct {
	Schema             string            `json:"schema"`
	Protocol           string            `json:"protocol"`
	ScenarioID         string            `json:"scenario_id"`
	Decision           Decision          `json:"decision"`
	Reason             string            `json:"reason"`
	Digests            DigestBindings    `json:"digests"`
	Graph              Graph             `json:"graph"`
	Cold               RunReport         `json:"cold"`
	Warm               RunReport         `json:"warm"`
	Metrics            MetricPair        `json:"metrics"`
	Improvement        ImprovementClaim  `json:"improvement"`
	Unknowns           []UnknownEvidence `json:"unknowns"`
	Refutations        []string          `json:"refutations"`
	GeneratedArtifacts []ArtifactRecord  `json:"generated_artifacts"`
	Upstream           []UpstreamInput   `json:"upstream,omitempty"`
}

type ArtifactRecord struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	Digest string `json:"digest"`
}

type UpstreamInput struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Commit     string `json:"commit"`
	Digest     string `json:"digest"`
}

type ConformanceManifest struct {
	Schema    string             `json:"schema"`
	Scenarios []ManifestScenario `json:"scenarios"`
}

type ManifestScenario struct {
	Name     string   `json:"name"`
	Expected Decision `json:"expected"`
	Input    string   `json:"input"`
	Selected bool     `json:"selected"`
}

type ConformanceReport struct {
	Schema   string             `json:"schema"`
	Decision Decision           `json:"decision"`
	Cases    []Report           `json:"cases"`
	Summary  ConformanceSummary `json:"summary"`
}

type ConformanceSummary struct {
	TotalCases     int   `json:"total_cases"`
	Closed         int   `json:"closed"`
	Unknown        int   `json:"unknown"`
	Refuted        int   `json:"refuted"`
	TotalTests     int   `json:"total_tests"`
	SelectedTests  int   `json:"selected_tests"`
	ExecutedTests  int   `json:"executed_tests"`
	ReusedTests    int   `json:"reused_tests"`
	FailedTests    int   `json:"failed_tests"`
	UnknownTests   int   `json:"unknown_tests"`
	GeneratedBytes int64 `json:"generated_bytes"`
}
