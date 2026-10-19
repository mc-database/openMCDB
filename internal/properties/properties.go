package properties

import (
	"log"
	"os"
	"strconv"
)

var Version = "0.2.0"
var Addr = "0.0.0.0:8000"
var DBPath = "openmcdb.sqlite"

var SubmitRateLimit4h = intEnv("OPENMCDB_RATE_LIMIT_4H", 1)

var SubmitRateLimit24h = intEnv("OPENMCDB_RATE_LIMIT_24H", 4)

var TrustRealIP = boolEnv("OPENMCDB_TRUST_REAL_IP", false)

func intEnv(name string, fallback int) int {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("openmcdb: invalid %s=%q, using default %d", name, raw, fallback)
		return fallback
	}
	return n
}

func boolEnv(name string, fallback bool) bool {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		log.Printf("openmcdb: invalid %s=%q, using default %t", name, raw, fallback)
		return fallback
	}
	return v
}
