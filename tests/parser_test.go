package tests

import (
	"testing"

	"salland1-metadata-wordpress/internal/parser"
	"salland1-metadata-wordpress/pkg/utils"
)

func TestParseHosts(t *testing.T) {
	tests := []struct {
		name     string
		hosts    []string
		expected string
	}{
		{"Empty", []string{}, ""},
		{"One host", []string{"John"}, "John"},
		{"Two hosts", []string{"John", "Jane"}, "John en Jane"},
		{"Three hosts", []string{"John", "Jane", "Bob"}, "John, Jane en Bob"},
		{"Four hosts", []string{"John", "Jane", "Bob", "Alice"}, "John, Jane, Bob, & Team"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := utils.FormatHosts(tt.hosts)
			if result != tt.expected {
				t.Errorf("FormatHosts(%v) = %v, want %v", tt.hosts, result, tt.expected)
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name     string
		timeStr  string
		expected string
	}{
		{"Empty", "", ""},
		{"Valid RFC3339", "2025-11-29T17:00:00+01:00", "17:00"},
		{"Valid ISO", "2025-11-29T17:00:00", "17:00"},
		{"Invalid", "invalid-time", "invalid-time"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := utils.FormatTime(tt.timeStr)
			if result != tt.expected {
				t.Errorf("FormatTime(%v) = %v, want %v", tt.timeStr, result, tt.expected)
			}
		})
	}
}

func TestParseMissingFields(t *testing.T) {
	// Test with missing fields
	data := map[string]interface{}{
		"broadcast": map[string]interface{}{
			"current_show": map[string]interface{}{
				// Missing show field
			},
		},
	}

	result := parser.Parse(data)

	if result.ShowName != "" {
		t.Errorf("Expected empty ShowName, got %v", result.ShowName)
	}

	if len(result.HostNames) != 0 {
		t.Errorf("Expected empty HostNames, got %v", result.HostNames)
	}
}

func TestParseMalformedJSON(t *testing.T) {
	// Test with malformed data (non-map)
	data := "invalid"

	result := parser.Parse(data)

	if result.ShowName != "" {
		t.Errorf("Expected empty ShowName for malformed data, got %v", result.ShowName)
	}
}

func TestParseFullData(t *testing.T) {
	data := map[string]interface{}{
		"broadcast": map[string]interface{}{
			"current_show": map[string]interface{}{
				"show": map[string]interface{}{
					"name": "Test Show",
					"hosts": []interface{}{
						map[string]interface{}{"name": "John"},
						map[string]interface{}{"name": "Jane"},
					},
				},
				"fm_rds_pty":  "10",
				"fm_rds_ptyn": "Test",
			},
			"next_show": map[string]interface{}{
				"start": "2025-11-29T17:00:00+01:00",
				"show": map[string]interface{}{
					"name": "Next Show",
				},
			},
		},
	}

	result := parser.Parse(data)

	if result.ShowName != "Test Show" {
		t.Errorf("Expected ShowName 'Test Show', got %v", result.ShowName)
	}

	if len(result.HostNames) != 2 {
		t.Errorf("Expected 2 hosts, got %v", len(result.HostNames))
	}

	if result.FmRdsPty != "10" {
		t.Errorf("Expected FmRdsPty '10', got %v", result.FmRdsPty)
	}

	if result.FmRdsPtyn != "Test" {
		t.Errorf("Expected FmRdsPtyn 'Test', got %v", result.FmRdsPtyn)
	}
}
