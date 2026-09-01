package gooo

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type InventoryMetrics struct {
	GoFiles             int `json:"go_files"`
	GoooFiles           int `json:"gooo_files"`
	GoPhysicalLines     int `json:"go_physical_lines"`
	GoooPhysicalLines   int `json:"gooo_physical_lines"`
	Subdirectories      int `json:"subdirectories"`
	RegularFiles        int `json:"regular_files"`
}

type RuntimeMetric struct {
	WallMS      int64 `json:"wall_ms"`
	PeakRSSKiB  int64 `json:"peak_rss_kib"`
}

type RuntimeMetrics struct {
	Build        RuntimeMetric `json:"build"`
	Test         RuntimeMetric `json:"test"`
	Conformance  RuntimeMetric `json:"conformance"`
}

type TestMetrics struct {
	Total    int `json:"total"`
	Executed int `json:"executed"`
	Reused   int `json:"reused"`
	Failed   int `json:"failed"`
	Unknown  int `json:"unknown"`
}

type ArtifactMetrics struct {
	Count int   `json:"count"`
	Bytes int64 `json:"bytes"`
}

func MeasureInventory(root string, exclusions []string) (InventoryMetrics, error) {
	metrics := InventoryMetrics{}
	excluded := map[string]bool{}
	for _, path := range exclusions {
		excluded[filepath.Clean(path)] = true
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			if path != root {
				metrics.Subdirectories++
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if excluded[filepath.Clean(relative)] {
			return nil
		}
		metrics.RegularFiles++
		extension := filepath.Ext(path)
		if extension != ".go" && extension != ".gooo" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := physicalLines(data)
		if extension == ".go" {
			metrics.GoFiles++
			metrics.GoPhysicalLines += lines
		} else {
			metrics.GoooFiles++
			metrics.GoooPhysicalLines += lines
		}
		return nil
	})
	return metrics, err
}

func MeasureArtifacts(root string) (ArtifactMetrics, error) {
	metrics := ArtifactMetrics{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		metrics.Count++
		metrics.Bytes += info.Size()
		return nil
	})
	return metrics, err
}

func ParseRuntimeMetric(path string) (RuntimeMetric, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeMetric{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) != 2 {
		return RuntimeMetric{}, fmt.Errorf("runtime metric %s must contain seconds and peak RSS", path)
	}
	wallMS, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return RuntimeMetric{}, err
	}
	rss, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return RuntimeMetric{}, err
	}
	return RuntimeMetric{WallMS: wallMS, PeakRSSKiB: rss}, nil
}

func ParseGoTestJSON(path string) (TestMetrics, error) {
	file, err := os.Open(path)
	if err != nil {
		return TestMetrics{}, err
	}
	defer file.Close()
	var metrics TestMetrics
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event struct {
			Action  string `json:"Action"`
			Test    string `json:"Test"`
			Output  string `json:"Output"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			metrics.Unknown++
			continue
		}
		switch event.Action {
		case "run":
			if event.Test != "" {
				metrics.Total++
				metrics.Executed++
			}
		case "fail":
			if event.Test != "" {
				metrics.Failed++
			}
		case "output":
			if strings.Contains(event.Output, "(cached)") {
				metrics.Reused++
			}
		case "skip":
			if event.Test != "" {
				metrics.Unknown++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return TestMetrics{}, err
	}
	return metrics, nil
}

func physicalLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	lines := 0
	for _, byteValue := range data {
		if byteValue == '\n' {
			lines++
		}
	}
	if data[len(data)-1] != '\n' {
		lines++
	}
	return lines
}
