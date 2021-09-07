package main

import (
	"io"
	"net/http"
	"path"
	"strings"

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

type RootResponse struct {
	ID      string   `json:"id"`
	Res     []string `json:"available_resources"`
	Version string   `json:"version,omitempty"`
}

func defaultHandler(w http.ResponseWriter, r *http.Request) {

	respondJSONError(w, http.StatusInternalServerError, "resource unknown")
	//return
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	rr := RootResponse{ID: "SAGE object store (node data)",
		Res:     []string{"api/v1/"},
		Version: version}
	respondJSON(w, http.StatusOK, &rr)
	//return
}

func getFileRequest(w http.ResponseWriter, r *http.Request) {

	pathParams := mux.Vars(r)
	sf := SageFileID{}
	sf.JobID = pathParams["jobID"]
	sf.TaskID = pathParams["taskID"]
	sf.NodeID = pathParams["nodeID"]

	tfarray := strings.SplitN(pathParams["timestampAndFilename"], "-", 2)
	sf.Timestamp = tfarray[0]
	sf.Filename = tfarray[1]

	// TODO check permissions here

	//w.Write([]byte("test"))
	//respondJSON(w, http.StatusOK, &sf)

	filename := sf.Timestamp + "-" + sf.Filename
	s3key := path.Join(s3rootFolder, sf.JobID, sf.TaskID, sf.NodeID, filename)

	objectInput := s3.GetObjectInput{
		Bucket: &s3bucket,
		Key:    &s3key,
	}

	out, err := svc.GetObject(&objectInput)
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, "Error getting data, svc.GetObject returned: %s", err.Error())
		return
	}
	defer out.Body.Close()

	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
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
