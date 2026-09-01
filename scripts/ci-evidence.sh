#!/usr/bin/env bash
set -euo pipefail

repo_root=${1:?repository root}
artifact_root=${2:?caller-owned artifact root}
output=${3:?evidence output path}
compile_time=${4:?compile timing file}
build_time=${5:?build timing file}
test_time=${6:?test timing file}
conformance_time=${7:?conformance timing file}
integration_time=${8:?integration timing file}
conformance_report=${9:?conformance report}

read -r compile_ms compile_rss < "$compile_time"
read -r build_ms build_rss < "$build_time"
read -r test_ms test_rss < "$test_time"
read -r conformance_ms conformance_rss < "$conformance_time"
read -r integration_ms integration_rss < "$integration_time"

sum_lines() {
  local pattern=$1
  local total=0
  while IFS= read -r -d '' file; do
    total=$((total + $(wc -l < "$file")))
  done < <(find "$repo_root" -type f $pattern -not -path "$repo_root/.git/*" -not -name README.md -print0)
  echo "$total"
}

count_files() {
  local pattern=$1
  find "$repo_root" -type f $pattern -not -path "$repo_root/.git/*" -not -name README.md -print | wc -l | tr -d ' '
}

go_files=$(count_files -name '*.go')
gooo_files=$(count_files -name '*.gooo')
go_lines=$(sum_lines -name '*.go')
gooo_lines=$(sum_lines -name '*.gooo')
regular_files=$(find "$repo_root" -type f -not -path "$repo_root/.git/*" -not -name README.md -print | wc -l | tr -d ' ')
subdirectories=$(find "$repo_root" -type d -not -path "$repo_root/.git" -not -path "$repo_root/.git/*" -print | wc -l | tr -d ' ')
generated_files=$(find "$artifact_root/generated" -type f -name '*.go' -print | wc -l | tr -d ' ')
generated_bytes=$(find "$artifact_root/generated" -type f -name '*.go' -exec wc -c {} + | awk 'END {print $1 + 0}')
repository_writes=$(git -C "$repo_root" status --porcelain | wc -l | tr -d ' ')
lock_digest=$(sha256sum "$repo_root/contracts/upstream-lock-v1.json" | awk '{print "sha256:" $1}')

jq -n \
  --argjson compile_ms "$compile_ms" --argjson compile_rss "$compile_rss" \
  --argjson build_ms "$build_ms" --argjson build_rss "$build_rss" \
  --argjson test_ms "$test_ms" --argjson test_rss "$test_rss" \
  --argjson conformance_ms "$conformance_ms" --argjson conformance_rss "$conformance_rss" \
  --argjson integration_ms "$integration_ms" --argjson integration_rss "$integration_rss" \
  --argjson go_files "$go_files" --argjson gooo_files "$gooo_files" \
  --argjson go_lines "$go_lines" --argjson gooo_lines "$gooo_lines" \
  --argjson regular_files "$regular_files" --argjson subdirectories "$subdirectories" \
  --argjson generated_files "$generated_files" --argjson generated_bytes "$generated_bytes" \
  --argjson repository_writes "$repository_writes" --arg lock_digest "$lock_digest" \
  --slurpfile conformance "$conformance_report" \
  '{
    schema: "gooo/incremental-module-compiler/ci-evidence/v1",
    verification_authority: "GITHUB_ACTIONS",
    root_readme_excluded: true,
    repository_writes: $repository_writes,
    inventory: {go_files: $go_files, go_physical_lines: $go_lines, gooo_files: $gooo_files, gooo_physical_lines: $gooo_lines, regular_files: $regular_files, subdirectories: $subdirectories},
    generated_artifacts: {files: $generated_files, bytes: $generated_bytes},
    stages: {
      compile: {wall_ms: $compile_ms, peak_rss_kib: $compile_rss},
      build: {wall_ms: $build_ms, peak_rss_kib: $build_rss},
      test: {wall_ms: $test_ms, peak_rss_kib: $test_rss},
      conformance: {wall_ms: $conformance_ms, peak_rss_kib: $conformance_rss},
      integration: {wall_ms: $integration_ms, peak_rss_kib: $integration_rss}
    },
    tests: ($conformance[0].summary | {total: .total_tests, selected: .selected_tests, executed: .executed_tests, reused: .reused_tests, failed: .failed_tests, unknown: .unknown_tests}),
    corpus: ($conformance[0].summary | {total_cases, closed, unknown, refuted}),
    upstream_lock_digest: $lock_digest
  }' > "$output"
