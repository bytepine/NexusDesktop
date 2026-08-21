// Copyright byteyang. All Rights Reserved.

package unreal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadAuthToken 在 /status 探活成功后按 mcpPort 读取 {Temp}/NexusLink/{PID}.json。
func ReadAuthToken(mcpPort int) string {
	dir := filepath.Join(os.TempDir(), "NexusLink")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		base := strings.TrimSuffix(name, ".json")
		if _, err := strconv.Atoi(base); err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var obj map[string]interface{}
		if json.Unmarshal(raw, &obj) != nil {
			continue
		}
		port, _ := obj["mcpPort"].(float64)
		token, _ := obj["authToken"].(string)
		if int(port) == mcpPort && token != "" {
			return token
		}
	}
	return ""
}
