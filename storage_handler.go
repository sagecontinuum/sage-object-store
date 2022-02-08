package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

type StorageHandler struct {
	S3API         s3iface.S3API
	S3Bucket      string
	S3RootFolder  string
	Authenticator Authenticator
	Logger        *log.Logger
}

type StorageFile struct {
	JobID     string
	TaskID    string
	NodeID    string
	Filename  string
	Timestamp time.Time
}

func (h *StorageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Logger != nil {
		h.Logger.Printf("storage handler: %s %s", r.Method, r.URL)
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")

	switch r.Method {
	case http.MethodOptions:
		// what response goes here?
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
		respondJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.handleAuth(w, r, sf); err != nil {
		return
	}

	s3key := h.s3KeyForFileID(sf)

	headObjectInput := s3.HeadObjectInput{
		Bucket: &h.S3Bucket,
		Key:    &s3key,
	}

	hoo, err := h.S3API.HeadObjectWithContext(r.Context(), &headObjectInput)
	if err != nil {
		switch aerr := err.(type) {
		case awserr.Error:
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
				respondJSONError(w, http.StatusNotFound, "bucket not found: %s", err.Error())
			case s3.ErrCodeNoSuchKey:
				respondJSONError(w, http.StatusNotFound, "file not found: %s", err.Error())
			default:
				respondJSONError(w, http.StatusInternalServerError, "Error getting data, HeadObjectWithContext returned: %s", aerr.Code())
			}
		default:
			respondJSONError(w, http.StatusInternalServerError, "Error getting data, HeadObjectWithContext returned: %s", err.Error())
		}
		return
	}

	if hoo.ContentLength != nil {
		w.Header().Add("Content-Length", fmt.Sprintf("%d", *hoo.ContentLength))
	}

	respondJSON(w, http.StatusOK, &hoo)
}

func (h *StorageHandler) handleGET(w http.ResponseWriter, r *http.Request) {
	sf, err := getRequestFileID(r)
	if err != nil {
		respondJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.handleAuth(w, r, sf); err != nil {
		return
	}

	s3key := h.s3KeyForFileID(sf)

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

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", sf.Filename))

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

func (h *StorageHandler) handleAuth(w http.ResponseWriter, r *http.Request, f *StorageFile) error {
	username, password, hasAuth := r.BasicAuth()
	if h.Authenticator.Authorized(f, username, password, hasAuth) {
		return nil
	}
	w.Header().Set("WWW-Authenticate", "Basic domain=storage.sagecontinuum.org")
	respondJSONError(w, http.StatusUnauthorized, "not authorized")
	return fmt.Errorf("not authorized")
}

func (h *StorageHandler) s3KeyForFileID(f *StorageFile) string {
	return path.Join(h.S3RootFolder, f.JobID, f.TaskID, f.NodeID, f.Filename)
}

func parseNanosecondTimestamp(s string) (time.Time, error) {
	nsec, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, nsec), nil
}

func extractTimestampFromFilename(s string) (time.Time, error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) < 2 {
		return time.Time{}, fmt.Errorf("missing dash separator in filename")
	}
	return parseNanosecondTimestamp(parts[0])
}

func getRequestFileID(r *http.Request) (*StorageFile, error) {
	// url format is {jobID}/{taskID}/{nodeID}/{timestampAndFilename}
	parts := strings.SplitN(r.URL.Path, "/", 4)
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid path: %q", r.URL.Path)
	}

	jobID := parts[0]
	taskID := parts[1]
	nodeID := parts[2]
	filename := parts[3]

	switch {
	case jobID == "":
		return nil, fmt.Errorf("job must be nonempty")
	case taskID == "":
		return nil, fmt.Errorf("task must be nonempty")
	case nodeID == "":
		return nil, fmt.Errorf("node must be nonempty")
	case filename == "":
		return nil, fmt.Errorf("filename must be nonempty")
	}

	timestamp, err := extractTimestampFromFilename(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to extract timestamp from filename: %s", err.Error())
	}

	return &StorageFile{
		JobID:     jobID,
		TaskID:    taskID,
		NodeID:    nodeID,
		Filename:  filename,
		Timestamp: timestamp,
	}, nil
}
