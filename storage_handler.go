package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

type Storage interface {
	GetObjectInfo(ctx context.Context, key string) (*s3.HeadObjectOutput, error)
	GetObjectPresignedURL(ctx context.Context, key string) (string, error)
}

type S3Storage struct {
	Bucket string
	S3     s3iface.S3API
}

func (s *S3Storage) GetObjectInfo(ctx context.Context, key string) (*s3.HeadObjectOutput, error) {
	return s.S3.HeadObjectWithContext(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	})
}

func (s *S3Storage) GetObjectPresignedURL(ctx context.Context, key string) (string, error) {
	req, _ := s.S3.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	})
	presignedURL, err := req.Presign(60 * time.Second)
	if err != nil {
		return "", fmt.Errorf("error getting presigned url: %s", err.Error())
	}
	return presignedURL, nil
}

type StorageHandler struct {
	Storage       Storage
	RootFolder    string
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
	// Requests which provide an authorization header should be forwarded to the Django site so they can
	// start using the new auth system.
	if r.Header.Get("authorization") != "" {
		h.handleProxyToDjango(w, r)
		return
	}

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

func (h *StorageHandler) handleProxyToDjango(w http.ResponseWriter, r *http.Request) {
	h.log("client provided authorization. proxying to django downloads endpoint. %s", r.URL.Path)
	auth := r.Header.Get("authorization")
	proxyURL := "https://auth.sagecontinuum.org/downloads/" + r.URL.Path

	req, err := http.NewRequest(http.MethodGet, proxyURL, nil)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	// Forward Authorization to Django site.
	req.Header.Set("authorization", auth)

	// Create http Client which does not follow redirects.
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	// Forward
	w.Header().Set("location", resp.Header.Get("location"))
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (h *StorageHandler) handleHEAD(w http.ResponseWriter, r *http.Request) {
	sf, err := getRequestFileID(r)
	if err != nil {
		respondJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := h.Storage.GetObjectInfo(r.Context(), h.keyForFileID(sf))
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

	presignedURL, err := h.Storage.GetObjectPresignedURL(r.Context(), h.keyForFileID(sf))
	if err != nil {
		http.Error(w, fmt.Sprintf("error getting presigned url: %s", err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", sf.Filename))
	http.Redirect(w, r, presignedURL, http.StatusTemporaryRedirect)
}

func (h *StorageHandler) handleS3Error(w http.ResponseWriter, r *http.Request, err error) {
	switch err := err.(type) {
	case awserr.Error:
		switch err.Code() {
		case s3.ErrCodeNoSuchBucket, s3.ErrCodeNoSuchKey:
			h.log("%s %s -> %s: not found", r.Method, r.URL, r.RemoteAddr)
			respondJSONError(w, http.StatusNotFound, "not found")
			return
		}
	}

	// TODO(sean) hack to detect not found on head requests. should do integration testing against minio for these cases.
	if strings.HasPrefix(err.Error(), "NotFound") {
		h.log("%s %s -> %s: not found", r.Method, r.URL, r.RemoteAddr)
		respondJSONError(w, http.StatusNotFound, "not found")
		return
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

func (h *StorageHandler) keyForFileID(f *StorageFile) string {
	return path.Join(h.RootFolder, f.JobID, f.TaskID, f.NodeID, f.Filename)
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
