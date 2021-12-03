package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type ErrorStruct struct {
	Error string `json:"error,omitempty"`
}

func getEnvString(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		i, _ := strconv.Atoi(value)
		return i
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		if strings.ToLower(value) == "true" {
			return true
		}
		if value == "1" {
			return true
		}

		return false
	}
	return fallback
}

func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	// json.NewEncoder(w).Encode(data)
	if data != nil {
		s, err := json.MarshalIndent(data, "", "  ")
		if err == nil {
			w.Write(s)
		}
	}

}

func respondJSONError(w http.ResponseWriter, statusCode int, msg string, args ...interface{}) {

	if msg != "" {
		errorStr := fmt.Sprintf(msg, args...)
		log.Printf("Reply to client: %s", errorStr)
		respondJSON(w, statusCode, ErrorStruct{Error: errorStr})
		return
	}
	respondJSON(w, statusCode, nil)
}
