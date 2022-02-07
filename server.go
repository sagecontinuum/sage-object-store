package main

import (
	"flag"
	"fmt"
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

func createRouter(handler *SageStorageHandler) *mux.Router {
	router := mux.NewRouter()

	router.HandleFunc("/", rootHandler).Methods(http.MethodGet, http.MethodOptions)

	router.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id": "SAGE object store (node data)",\n"available_resources":["data"]}`)
	})

	// TODO move vars in URL in SageStorageHandler
	// GET /data/
	router.Handle("/api/v1/data/{jobID}/{taskID}/{nodeID}/{timestampAndFilename}", handler).Methods(http.MethodGet, http.MethodHead, http.MethodOptions)

	// http.Handle("/metrics", promhttp.Handler())
	router.Handle("/metrics", promhttp.Handler()).Methods(http.MethodGet)

	router.NotFoundHandler = http.HandlerFunc(defaultHandler)
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

	handler := &SageStorageHandler{
		S3API:         s3.New(session),
		S3Bucket:      s3bucket,
		S3RootFolder:  s3rootFolder,
		Authenticator: TableAuthenticator,
	}

	r := createRouter(handler)

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, r))
}
