package modrinth

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

func projectStatus(s string) string {
	switch s {
	case "approved", "rejected", "draft", "unlisted", "processing", "withheld", "scheduled", "private", "unknown":
		return s
	default:
		return "unknown"
	}
}

func versionStatus(s string) string {
	switch s {
	case "listed", "archived", "draft", "unlisted", "scheduled", "unknown":
		return s
	default:
		return "unknown"
	}
}

func versionType(s string) string {
	switch s {
	case "release", "beta", "alpha", "unknown":
		return s
	default:
		return "unknown"
	}
}

func gameVersionType(s string) string {
	switch s {
	case "release", "snapshot", "alpha", "beta", "unknown":
		return s
	default:
		return "unknown"
	}
}

func environment(s string) string {
	switch s {
	case "client_and_server", "client_only", "client_only_server_optional",
		"server_only", "server_only_client_optional", "client_or_server",
		"client_or_server_prefers_both", "singleplayer_only", "dedicated_server_only",
		"unknown":
		return s
	default:
		return "unknown"
	}
}

func depKind(s string) string {
	switch s {
	case "required", "optional", "incompatible", "embedded", "unknown":
		return s
	default:
		return "unknown"
	}
}

func monetization(s string) string {
	switch s {
	case "monetized", "demonetized", "force-demonetized", "unknown":
		return s
	default:
		return "unknown"
	}
}

func sideTypesReviewStatus(s string) string {
	switch s {
	case "reviewed", "pending", "unknown":
		return s
	default:
		return "unknown"
	}
}

func serverRegion(s string) (string, bool) {
	switch s {
	case "us_east", "us_west", "europe", "asia", "australia", "south_america", "middle_east", "russia":
		return s, true
	default:
		return "", false
	}
}

func game(s string) (string, bool) {
	switch s {
	case "minecraft-java", "minecraft-bedrock":
		return s, true
	default:
		return "", false
	}
}

func userRole(s string) string {
	switch s {
	case "developer", "moderator", "admin", "unknown":
		return s
	default:
		return "unknown"
	}
}

func projectType(s string) string {
	switch s {
	case "mod", "modpack", "resourcepack", "shader", "datapack", "plugin", "minecraft_java_server", "project", "unknown":
		return s
	default:
		return "unknown"
	}
}

func categoryNamespaces(projectTypes []string) []string {
	namespaces := make([]string, 0, len(projectTypes))
	for _, pt := range projectTypes {
		switch pt {
		case "plugin", "datapack":
			namespaces = append(namespaces, "mod")
		default:
			namespaces = append(namespaces, pt)
		}
	}
	slices.Sort(namespaces)
	return slices.Compact(namespaces)
}

func disclosureKind(s string) (string, bool) {
	switch strings.ToLower(s) {
	case "ai_content", "ai_content_code", "ai_content_assets", "ai_content_text",
		"ai_functionality", "advertisements", "epilepsy_triggers",
		"system_interactions", "telemetry", "telemetry_opt_in", "telemetry_opt_out",
		"telemetry_always_active", "derivative_work", "paid_features", "archived":
		return strings.ToLower(s), true
	default:
		return "", false
	}
}

func mapEnum(coverage map[string]int, mapper, raw string, mapValue func(string) string) string {
	mapped := mapValue(raw)
	if mapper == "environment" && raw == "" {
		if coverage != nil {
			coverage["environments_missing"]++
		}
		return mapped
	}
	if mapped == "unknown" && raw != "unknown" {
		recordUnmapped(coverage, mapper, raw)
	}
	return mapped
}

func mapOptionalValue(coverage map[string]int, mapper, raw string, mapValue func(string) (string, bool)) (string, bool) {
	mapped, ok := mapValue(raw)
	if !ok {
		recordUnmapped(coverage, mapper, raw)
	}
	return mapped, ok
}

func recordUnmapped(coverage map[string]int, mapper, raw string) {
	recordCoverageValue(coverage, "unmapped", mapper, raw, 1)
}

const coverageValueLimit = 256

func recordCoverageValue(coverage map[string]int, namespace, mapper, raw string, count int) {
	if coverage == nil || count <= 0 {
		return
	}
	prefix := namespace + ":" + mapper + ":"
	coverage[namespace+":"+mapper] += count
	value := []rune(raw)
	if len(value) > 96 {
		hash := sha256.Sum256([]byte(raw))
		raw = string(value[:96]) + fmt.Sprintf("…#%x", hash[:8])
	}
	key := prefix + strconv.QuoteToASCII(raw)
	if _, exists := coverage[key]; !exists {
		keys := 0
		for k := range coverage {
			if strings.HasPrefix(k, prefix) {
				keys++
			}
		}
		if keys >= coverageValueLimit {
			coverage[prefix+"<overflow>"] += count
			return
		}
	}
	coverage[key] += count
}

func boolPtrDeref(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}
