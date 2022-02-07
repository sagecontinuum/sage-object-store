package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type ErrorStruct struct {
	Error string `json:"error,omitempty"`
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
		// log.Printf("Reply to client: %s", errorStr)
		respondJSON(w, statusCode, ErrorStruct{Error: errorStr})
		return
	}
	respondJSON(w, statusCode, nil)
}
