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
	h.log("%s %s -> %s: serving", r.Method, r.URL, r.RemoteAddr)

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

	resp, err := h.S3API.HeadObjectWithContext(r.Context(), &headObjectInput)
	if err != nil {
		h.handleS3Error(w, r, err)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", sf.Filename))

	if resp.ContentLength != nil {
		w.Header().Add("Content-Length", fmt.Sprintf("%d", *resp.ContentLength))
	}
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

	resp, err := h.S3API.GetObjectWithContext(r.Context(), &objectInput)
	if err != nil {
		h.handleS3Error(w, r, err)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", sf.Filename))

	if resp.ContentLength != nil {
		w.Header().Set("Content-Length", strconv.FormatInt(*resp.ContentLength, 10))
	}

	w.WriteHeader(http.StatusOK)

	written, err := io.Copy(w, resp.Body)
	switch {
	case resp.ContentLength != nil && written != *resp.ContentLength:
		h.log("%s %s -> %s: partial: %d of %d bytes written", r.Method, r.URL, r.RemoteAddr, written, *resp.ContentLength)
	case err != nil:
		h.log("%s %s -> %s: write error %s", r.Method, r.URL, r.RemoteAddr, err.Error())
	default:
		h.log("%s %s -> %s: ok: %d bytes written", r.Method, r.URL, r.RemoteAddr, written)
	}
	// track bytes written, even if write fails
	fileDownloadByteSize.Add(float64(written))
}

func (h *StorageHandler) handleS3Error(w http.ResponseWriter, r *http.Request, err error) {
	switch err := err.(type) {
	case awserr.Error:
		switch err.Code() {
		case s3.ErrCodeNoSuchBucket:
			h.log("%s %s -> %s: bucket not found", r.Method, r.URL, r.RemoteAddr)
			respondJSONError(w, http.StatusNotFound, "bucket not found")
			return
		case s3.ErrCodeNoSuchKey:
			h.log("%s %s -> %s: file not found", r.Method, r.URL, r.RemoteAddr)
			respondJSONError(w, http.StatusNotFound, "file not found")
			return
		}
	}
	h.log("%s %s -> %s: s3 error: %s", r.Method, r.URL, r.RemoteAddr, err.Error())
	respondJSONError(w, http.StatusInternalServerError, "internal server error with S3 request: %s", err.Error())
}

func (h *StorageHandler) handleAuth(w http.ResponseWriter, r *http.Request, f *StorageFile) error {
	username, password, hasAuth := r.BasicAuth()
	if h.Authenticator.Authorized(f, username, password, hasAuth) {
		return nil
	}
	h.log("%s %s -> %s: not authorized", r.Method, r.URL, r.RemoteAddr)
	w.Header().Set("WWW-Authenticate", "Basic domain=storage.sagecontinuum.org")
	respondJSONError(w, http.StatusUnauthorized, "not authorized")
	return fmt.Errorf("not authorized")
}

func (h *StorageHandler) s3KeyForFileID(f *StorageFile) string {
	return path.Join(h.S3RootFolder, f.JobID, f.TaskID, f.NodeID, f.Filename)
}

func (h *StorageHandler) log(format string, v ...interface{}) {
	if h.Logger == nil {
		return
	}
	h.Logger.Printf(format, v...)
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
