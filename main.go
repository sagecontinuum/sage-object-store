package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

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
		type response struct {
			ID      string   `json:"id"`
			Res     []string `json:"available_resources"`
			Version string   `json:"version,omitempty"`
		}

		respondJSON(w, http.StatusOK, &response{
			ID:      "SAGE object store (node data)",
			Res:     []string{"api/v1/"},
			Version: "[[VERSION]]",
		})
	})

	// add discovery endpoint to show what's under /api/v1/
	router.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		type response struct {
			ID  string   `json:"id"`
			Res []string `json:"available_resources"`
		}

		respondJSON(w, http.StatusOK, &response{
			ID:  "SAGE object store (node data)",
			Res: []string{"data/"},
		})
	})

	router.Handle("/api/v1/data/", http.StripPrefix("/api/v1/data/", handler))

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

	nodes, err := GetNodeTableFromURL("https://api.sagecontinuum.org/production")
	if err != nil {
		log.Fatalf("failed to load nodes table: %s", err)
	}

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
		Username:                  os.Getenv("policyRestrictedUsername"),
		Password:                  os.Getenv("policyRestrictedPassword"),
		Nodes:                     nodes,
		RestrictedTasksSubstrings: strings.Split(os.Getenv("policyRestrictedTaskSubstrings"), ","),
	})
	// TODO(sean) sync with production api periodically

	handler := &StorageHandler{
		S3API:         s3.New(session),
		S3Bucket:      s3bucket,
		S3RootFolder:  s3rootFolder,
		Authenticator: TableAuthenticator,
		Logger:        log.Default(),
	}

	r := createRouter(handler)

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, r))
}
