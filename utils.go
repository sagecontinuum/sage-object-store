package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if data == nil {
		return
	}
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	e.Encode(data)
}

func respondJSONError(w http.ResponseWriter, statusCode int, msg string, args ...interface{}) {
	type resp struct {
		Error string `json:"error,omitempty"`
	}
	if msg == "" {
		respondJSON(w, statusCode, nil)
	}
	respondJSON(w, statusCode, &resp{
		Error: fmt.Sprintf(msg, args...),
	})
}
