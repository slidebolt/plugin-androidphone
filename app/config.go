package app

import "os"

type Config struct {
	ProjectID       string
	CredentialsJSON string
	BaseURL         string
}

func loadConfigFromEnv() Config {
	return Config{
		ProjectID:       os.Getenv("ANDROIDPHONE_FCM_PROJECT_ID"),
		CredentialsJSON: os.Getenv("ANDROIDPHONE_FCM_CREDENTIALS_JSON"),
		BaseURL:         getenv("ANDROIDPHONE_FCM_BASE_URL", "https://fcm.googleapis.com"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
