package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func createRouter(handler *StorageHandler) *mux.Router {
	router := mux.NewRouter()

	// add discovery endpoint to show what's under /
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		type resp struct {
			ID      string   `json:"id"`
			Res     []string `json:"available_resources"`
			Version string   `json:"version,omitempty"`
		}

		respondJSON(w, http.StatusOK, &resp{
			ID:      "SAGE object store (node data)",
			Res:     []string{"api/v1/"},
			Version: "[[VERSION]]",
		})
	})

	// add discovery endpoint to show what's under /api/v1/
	router.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		type resp struct {
			ID  string   `json:"id"`
			Res []string `json:"available_resources"`
		}

		respondJSON(w, http.StatusOK, &resp{
			ID:  "SAGE object store (node data)",
			Res: []string{"data/"},
		})
	})

	// TODO move vars in URL in StorageHandler
	// GET /data/
	router.Handle("/api/v1/data/{jobID}/{taskID}/{nodeID}/{timestampAndFilename}", handler).Methods(http.MethodGet, http.MethodHead, http.MethodOptions)

	// add prometheus metrics endpoint
	router.Handle("/metrics", promhttp.Handler()).Methods(http.MethodGet)

	router.Use(mux.CORSMethodMiddleware(router))
	return router
}

func mustGetenv(key string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		log.Fatalf("env %q is required", key)
	}
	return val
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	flag.Parse()

	s3Endpoint := mustGetenv("s3Endpoint")
	s3accessKeyID := mustGetenv("s3accessKeyID")
	s3secretAccessKey := mustGetenv("s3secretAccessKey")
	s3bucket := mustGetenv("s3bucket")
	s3rootFolder := os.Getenv("s3rootFolder")

	session := session.Must(session.NewSession(&aws.Config{
		Credentials:      credentials.NewStaticCredentials(s3accessKeyID, s3secretAccessKey, ""),
		Endpoint:         aws.String(s3Endpoint),
		Region:           aws.String("us-west-2"),
		DisableSSL:       aws.Bool(false),
		S3ForcePathStyle: aws.Bool(true),
	}))

	TableAuthenticator := &TableAuthenticator{}

	TableAuthenticator.UpdateConfig(&TableAuthenticatorConfig{
		Username: os.Getenv("policyRestrictedUsername"),
		Password: os.Getenv("policyRestrictedPassword"),
	})
	// TODO(sean) dispatch sync with production sheet

	handler := &StorageHandler{
		S3API:         s3.New(session),
		S3Bucket:      s3bucket,
		S3RootFolder:  s3rootFolder,
		Authenticator: TableAuthenticator,
	}

	r := createRouter(handler)

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, r))
}
