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

type StorageHandler struct {
	S3API         s3iface.S3API
	S3Bucket      string
	S3RootFolder  string
	Authenticator Authenticator
}

type StorageFile struct {
	JobID     string
	TaskID    string
	NodeID    string
	Timestamp string
	Filename  string
}

func getRequestFileID(r *http.Request) (*StorageFile, error) {
	vars := mux.Vars(r)

	tfarray := strings.SplitN(vars["timestampAndFilename"], "-", 2)
	if len(tfarray) < 2 {
		return nil, fmt.Errorf("filename has wrong format, dash expected, got %s (sf.JobID: %s)", vars["timestampAndFilename"], vars["jobID"])
	}

	return &StorageFile{
		JobID:     vars["jobID"],
		TaskID:    vars["taskID"],
		NodeID:    vars["nodeID"],
		Timestamp: tfarray[0],
		Filename:  tfarray[1],
	}, nil
}

func filenameForFileID(sf *StorageFile) string {
	return sf.Timestamp + "-" + sf.Filename
}

func (h *StorageHandler) s3KeyForFileID(sf *StorageFile) string {
	return path.Join(h.S3RootFolder, sf.JobID, sf.TaskID, sf.NodeID, filenameForFileID(sf))
}

func (h *StorageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// log request (debug mode only...)
	// log.Printf("%s %s", r.Method, r.URL)

	// storage is always read only, so we allow any origin
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// dispatch request to specific handler func
	switch r.Method {
	case http.MethodOptions:
		// TODO(sean) implement OPTIONS response
	case http.MethodHead:
		h.handleHEAD(w, r)
	case http.MethodGet:
		h.handleGET(w, r)
	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func (h *StorageHandler) handleHEAD(w http.ResponseWriter, r *http.Request) {
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

func (h *StorageHandler) handleGET(w http.ResponseWriter, r *http.Request) {
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
