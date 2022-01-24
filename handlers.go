package main

import (
	"io"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
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
	//return
}

func rootHandler(w http.ResponseWriter, r *http.Request) {

	rr := RootResponse{
		ResourceResponse: ResourceResponse{
			ID:  "SAGE object store (node data)",
			Res: []string{"api/v1/"},
		},
		Version: version}

	//w.Header().Set("Access-Control-Allow-Origin", "*")
	respondJSON(w, http.StatusOK, &rr)
	//return
}

func headFileRequest(w http.ResponseWriter, r *http.Request) {
	//w.Header().Set("Access-Control-Allow-Origin", "*")
	ctx := r.Context()
	pathParams := mux.Vars(r)
	sf := SageFileID{}
	sf.JobID = pathParams["jobID"]
	sf.TaskID = pathParams["taskID"]
	sf.NodeID = pathParams["nodeID"]

	timestampAndFilename := pathParams["timestampAndFilename"]
	tfarray := strings.SplitN(timestampAndFilename, "-", 2)
	if len(tfarray) < 2 {
		respondJSONError(w, http.StatusInternalServerError, "Filename has wrong format, dash expected, got %s (sf.JobID: %s)", timestampAndFilename, sf.JobID)
		return
	}
	sf.Timestamp = tfarray[0]
	sf.Filename = tfarray[1]

	// TODO check permissions here

	//w.Write([]byte("test"))
	//respondJSON(w, http.StatusOK, &sf)

	filename := sf.Timestamp + "-" + sf.Filename
	s3key := path.Join(s3rootFolder, sf.JobID, sf.TaskID, sf.NodeID, filename)

	headObjectInput := s3.HeadObjectInput{
		Bucket: &s3bucket,
		Key:    &s3key,
	}

	hoo, err := svc.HeadObjectWithContext(ctx, &headObjectInput)
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
			respondJSONError(w, http.StatusInternalServerError, "Error getting data, svc.HeadObjectWithContext returned: %s", aerr.Code())
			return
		}

		respondJSONError(w, http.StatusInternalServerError, "Error getting data, svc.HeadObjectWithContext returned: %s", err.Error())
		return
	}

	respondJSON(w, http.StatusOK, &hoo)

}

func getFileRequest(w http.ResponseWriter, r *http.Request) {

	//w.Header().Set("Access-Control-Allow-Origin", "*")

	pathParams := mux.Vars(r)
	sf := SageFileID{}
	sf.JobID = pathParams["jobID"]
	sf.TaskID = pathParams["taskID"]
	sf.NodeID = pathParams["nodeID"]

	tfarray := strings.SplitN(pathParams["timestampAndFilename"], "-", 2)
	if len(tfarray) < 2 {
		respondJSONError(w, http.StatusInternalServerError, "Filename has wrong format, dash expected")
		return
	}
	sf.Timestamp = tfarray[0]
	sf.Filename = tfarray[1]

	basic_auth_ok := false
	username, password, ok := r.BasicAuth()
	if ok {
		if username != policyRestrictedUsername || password != policyRestrictedPassword {
			w.Header().Set("WWW-Authenticate", "Basic domain=storage.sagecontinuum.org")
			respondJSONError(w, http.StatusUnauthorized, "not authorized")
			return
		}
		basic_auth_ok = true
	}
	if !basic_auth_ok {

		_, isRestrictedNode := policyRestrictedNodes[strings.ToLower(sf.NodeID)]

		if isRestrictedNode {

			for _, s := range policyRestrictedTaskSubstrings {
				if strings.Contains(sf.TaskID, s) {
					//w.Header().Set("WWW-Authenticate", "Basic")
					w.Header().Set("WWW-Authenticate", "Basic domain=storage.sagecontinuum.org")
					respondJSONError(w, http.StatusUnauthorized, "not authorized")
					return
				}

			}
		}

		// check if node is outside of commission time

		timeFull, err := strconv.ParseInt(sf.Timestamp, 10, 64)
		if err != nil {

			respondJSONError(w, http.StatusBadRequest, "Timestamp in filename has wrong format: %s", err.Error())
			return
		}

		timestamp := time.Unix(timeFull/1e9, timeFull%1e9)

		cdate, ok := GetNodeCommissionDate(strings.ToLower(sf.NodeID))
		if !ok {
			w.Header().Set("WWW-Authenticate", "Basic domain=storage.sagecontinuum.org")
			respondJSONError(w, http.StatusUnauthorized, "not authorized, node not commissioned")
			return
		}

		if timestamp.Before(cdate) {
			w.Header().Set("WWW-Authenticate", "Basic domain=storage.sagecontinuum.org")
			respondJSONError(w, http.StatusUnauthorized, "not authorized, date before commission date")
			return
		}
	}

	// TODO check permissions here

	//w.Write([]byte("test"))
	//respondJSON(w, http.StatusOK, &sf)

	filename := sf.Timestamp + "-" + sf.Filename
	s3key := path.Join(s3rootFolder, sf.JobID, sf.TaskID, sf.NodeID, filename)

	objectInput := s3.GetObjectInput{
		Bucket: &s3bucket,
		Key:    &s3key,
	}
	ctx := r.Context()
	out, err := svc.GetObjectWithContext(ctx, &objectInput)
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, "Error getting data, svc.GetObject returned: %s", err.Error())
		return
	}
	defer out.Body.Close()

	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	contentLength := *out.ContentLength
	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	//w.Header().Set("Content-Length", FileSize)

	buffer := make([]byte, 1024*1024)
	w.WriteHeader(http.StatusOK)
	for {
		n, err := out.Body.Read(buffer)
		if err != nil {

			if err == io.EOF {
				w.Write(buffer[:n]) //should handle any remainding bytes.
				fileDownloadByteSize.Add(float64(n))
				break
			}

			respondJSONError(w, http.StatusInternalServerError, "Error getting data: %s", err.Error())
			return
		}
		w.Write(buffer[0:n])
		fileDownloadByteSize.Add(float64(n))
	}

	//respondJSONError(w, http.StatusInternalServerError, "resource unknown")
	//return
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Do stuff here
		log.Println(r.RequestURI)

		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Call the next handler, which can be another middleware in the chain, or the final handler.
		next.ServeHTTP(w, r)
	})
}
