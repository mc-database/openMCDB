package properties

import (
	"log"
	"os"
	"strconv"
)

var Version = "0.3.1"

type Config struct {
	Addr                     string
	DatabaseURL              string
	SurveySubmitRateLimit4h  int
	SurveySubmitRateLimit24h int
	TrustRealIP              bool
}

func Load() Config {
	return Config{
		Addr:                     stringEnv("OPENMCDB_ADDR", "0.0.0.0:8000"),
		DatabaseURL:              os.Getenv("OPENMCDB_DATABASE_URL"),
		SurveySubmitRateLimit4h:  intEnv("OPENMCDB_SURVEY_SUBMIT_RATE_LIMIT_4H", 1),
		SurveySubmitRateLimit24h: intEnv("OPENMCDB_SURVEY_SUBMIT_RATE_LIMIT_24H", 4),
		TrustRealIP:              boolEnv("OPENMCDB_TRUST_REAL_IP", false),
	}
}

func stringEnv(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

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
