package main

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/gorilla/mux"
)

type SageFileID struct {
	JobID     string
	TaskID    string
	NodeID    string
	Timestamp string
	Filename  string
}

type ResourceResponse struct {
	ID  string   `json:"id"`
	Res []string `json:"available_resources"`
}

type RootResponse struct {
	ResourceResponse
	Version string `json:"version,omitempty"`
}

func defaultHandler(w http.ResponseWriter, r *http.Request) {
	respondJSONError(w, http.StatusInternalServerError, "resource unknown")
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	rr := RootResponse{
		ResourceResponse: ResourceResponse{
			ID:  "SAGE object store (node data)",
			Res: []string{"api/v1/"},
		},
		Version: "[[VERSION]]",
	}
	respondJSON(w, http.StatusOK, &rr)
}

func getRequestFileID(r *http.Request) (*SageFileID, error) {
	vars := mux.Vars(r)

	tfarray := strings.SplitN(vars["timestampAndFilename"], "-", 2)
	if len(tfarray) < 2 {
		return nil, fmt.Errorf("filename has wrong format, dash expected, got %s (sf.JobID: %s)", vars["timestampAndFilename"], vars["jobID"])
	}

	return &SageFileID{
		JobID:     vars["jobID"],
		TaskID:    vars["taskID"],
		NodeID:    vars["nodeID"],
		Timestamp: tfarray[0],
		Filename:  tfarray[1],
	}, nil
}

type SageStorageHandler struct {
	S3API         s3iface.S3API
	S3Bucket      string
	S3RootFolder  string
	Authenticator Authenticator
}

func filenameForFileID(sf *SageFileID) string {
	return sf.Timestamp + "-" + sf.Filename
}

func (h *SageStorageHandler) s3KeyForFileID(sf *SageFileID) string {
	return path.Join(h.S3RootFolder, sf.JobID, sf.TaskID, sf.NodeID, filenameForFileID(sf))
}

func (h *SageStorageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// log request (debug mode only...)
	// log.Printf("%s %s", r.Method, r.URL)

	// storage is always read only, so we allow any origin
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// dispatch request to specific handler func
	switch r.Method {
	case http.MethodGet:
		getFileRequest(h, w, r)
	case http.MethodHead:
		headFileRequest(h, w, r)
	}
}

func headFileRequest(h *SageStorageHandler, w http.ResponseWriter, r *http.Request) {
	sf, err := getRequestFileID(r)
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s3key := h.s3KeyForFileID(sf)

	headObjectInput := s3.HeadObjectInput{
		Bucket: &h.S3Bucket,
		Key:    &s3key,
	}

	hoo, err := h.S3API.HeadObjectWithContext(r.Context(), &headObjectInput)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
				respondJSONError(w, http.StatusNotFound, "Bucket not found: %s", err.Error())
				return
			case s3.ErrCodeNoSuchKey, "NotFound":
				respondJSONError(w, http.StatusNotFound, "File not found: %s", err.Error())
				return
			}
			aerr.Code()
			respondJSONError(w, http.StatusInternalServerError, "Error getting data, HeadObjectWithContext returned: %s", aerr.Code())
			return
		}

		respondJSONError(w, http.StatusInternalServerError, "Error getting data, HeadObjectWithContext returned: %s", err.Error())
		return
	}

	respondJSON(w, http.StatusOK, &hoo)
}

func getFileRequest(h *SageStorageHandler, w http.ResponseWriter, r *http.Request) {
	sf, err := getRequestFileID(r)
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s3key := h.s3KeyForFileID(sf)

	username, password, hasAuth := r.BasicAuth()

	if !h.Authenticator.Authorized(sf, username, password, hasAuth) {
		w.Header().Set("WWW-Authenticate", "Basic domain=storage.sagecontinuum.org")
		respondJSONError(w, http.StatusUnauthorized, "not authorized")
		return
	}

	objectInput := s3.GetObjectInput{
		Bucket: &h.S3Bucket,
		Key:    &s3key,
	}

	out, err := h.S3API.GetObjectWithContext(r.Context(), &objectInput)
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, "Error getting data, GetObject returned: %s", err.Error())
		return
	}
	defer out.Body.Close()

	w.Header().Set("Content-Disposition", "attachment; filename="+filenameForFileID(sf))

	if out.ContentLength != nil {
		w.Header().Set("Content-Length", strconv.FormatInt(*out.ContentLength, 10))
	}

	w.WriteHeader(http.StatusOK)

	written, err := io.Copy(w, out.Body)
	fileDownloadByteSize.Add(float64(written))
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, "Error getting data: %s", err.Error())
		return
	}
}
