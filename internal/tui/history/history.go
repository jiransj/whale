package history

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/usewhale/whale/internal/core"
)

var environmentInventoryKeys = []string{
	"system:",
	"system：",
	"version:",
	"version：",
	"build:",
	"build：",
	"系统:",
	"系统：",
	"版本:",
	"版本：",
	"构建号:",
	"构建号：",
}

func IsEnvironmentInventoryBlock(text string) bool {
	block := strings.ToLower(strings.TrimSpace(text))
	if block == "" {
		return false
	}
	matched := 0
	for _, key := range environmentInventoryKeys {
		if strings.Contains(block, strings.ToLower(key)) {
			matched++
			if matched >= 2 {
				return true
			}
		}
	}
	return false
}

func SummarizeHydratedToolCall(call core.ToolCall) string {
	if strings.TrimSpace(call.Input) == "" {
		return call.Name
	}
	if call.Name == "shell_run" {
		var body map[string]any
		if err := json.Unmarshal([]byte(call.Input), &body); err == nil {
			if cmd, _ := body["command"].(string); strings.TrimSpace(cmd) != "" {
				return "Running " + strings.TrimSpace(cmd)
			}
		}
	}
	return fmt.Sprintf("%s: %s", call.Name, strings.TrimSpace(call.Input))
}
