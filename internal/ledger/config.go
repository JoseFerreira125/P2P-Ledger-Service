package ledger

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

type Config struct {
	InitialDifficulty            int
	BlockGenerationInterval      time.Duration
	DifficultyAdjustmentInterval int64
}

func LoadConfig() *Config {
	if err := godotenv.Load(); err != nil {
		logrus.Info("No .env file found, using environment variables and defaults")
	}

	return &Config{
		InitialDifficulty:            getEnvAsInt("INITIAL_DIFFICULTY", 4),
		BlockGenerationInterval:      getEnvAsDuration("BLOCK_GENERATION_INTERVAL_SECONDS", 10) * time.Second,
		DifficultyAdjustmentInterval: int64(getEnvAsInt("DIFFICULTY_ADJUSTMENT_INTERVAL_BLOCKS", 10)),
	}
}

func getEnv(key string, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return fallback
}

func getEnvAsDuration(key string, fallback time.Duration) time.Duration {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return time.Duration(value)
	}
	return fallback
}
