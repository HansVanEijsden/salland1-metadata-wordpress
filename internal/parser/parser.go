package parser

import (
	"strconv"
)

type ParsedData struct {
	ShowName     string
	HostNames    []string
	NextShowTime string
	NextShowName string
	FmRdsPty     string
	FmRdsPtyn    string
}

func Parse(data interface{}) ParsedData {
	result := ParsedData{}

	// Type assertion and safe navigation
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return result
	}

	// Navigate to broadcast
	broadcast, ok := dataMap["broadcast"].(map[string]interface{})
	if !ok {
		return result
	}

	// Parse current show
	currentShow, ok := broadcast["current_show"].(map[string]interface{})
	if ok {
		// Parse show name
		if show, ok := currentShow["show"].(map[string]interface{}); ok {
			if name, ok := show["name"].(string); ok {
				result.ShowName = name
			}
			// Parse hosts
			if hosts, ok := show["hosts"].([]interface{}); ok {
				for _, h := range hosts {
					if hostMap, ok := h.(map[string]interface{}); ok {
						if name, ok := hostMap["name"].(string); ok && name != "" {
							result.HostNames = append(result.HostNames, name)
						}
					}
				}
			}
		}

		// Parse FM RDS PTY
		if pty, ok := currentShow["fm_rds_pty"].(string); ok {
			result.FmRdsPty = pty
		} else if pty, ok := currentShow["fm_rds_pty"].(float64); ok {
			result.FmRdsPty = strconv.FormatFloat(pty, 'f', -1, 64)
		}
		// If it's an integer (JSON numbers are float64 in Go)
		if pty, ok := currentShow["fm_rds_pty"].(int); ok {
			result.FmRdsPty = strconv.Itoa(pty)
		}

		// Parse FM RDS PTYN
		if ptyn, ok := currentShow["fm_rds_ptyn"].(string); ok {
			result.FmRdsPtyn = ptyn
		}
	}

	// Parse next show
	nextShow, ok := broadcast["next_show"].(map[string]interface{})
	if ok {
		if start, ok := nextShow["start"].(string); ok {
			result.NextShowTime = start
		}
		if show, ok := nextShow["show"].(map[string]interface{}); ok {
			if name, ok := show["name"].(string); ok {
				result.NextShowName = name
			}
		}
	}

	return result
}
