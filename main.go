package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	flag.Parse()

	router := http.NewServeMux()

	auth := &TableAuthenticator{}
	go periodicallyUpdateAuthConfig(auth)

	session := session.Must(session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(
			mustGetenv("s3accessKeyID"),
			mustGetenv("s3secretAccessKey"),
			""),
		Endpoint:         aws.String(mustGetenv("s3Endpoint")),
		Region:           aws.String("us-west-2"),
		DisableSSL:       aws.Bool(false),
		S3ForcePathStyle: aws.Bool(true),
	}))

	router.Handle("/api/v1/data/", http.StripPrefix("/api/v1/data/", &StorageHandler{
		S3API:         s3.New(session),
		S3Bucket:      mustGetenv("s3bucket"),
		S3RootFolder:  mustGetenv("s3rootFolder"),
		Authenticator: auth,
		Logger:        log.Default(),
	}))

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

	// add prometheus metrics endpoint
	router.Handle("/metrics", promhttp.Handler())

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, router))
}

func periodicallyUpdateAuthConfig(auth *TableAuthenticator) {
	for {
		nodes, err := GetNodeTableFromURL("https://api.sagecontinuum.org/production")
		if err != nil {
			log.Printf("failed to get node table: %s", err.Error())
			time.Sleep(10 * time.Second)
			continue
		}

		auth.UpdateConfig(&TableAuthenticatorConfig{
			Username:                  os.Getenv("policyRestrictedUsername"),
			Password:                  os.Getenv("policyRestrictedPassword"),
			Nodes:                     nodes,
			RestrictedTasksSubstrings: []string{},
		})

		log.Printf("updated auth config")
		time.Sleep(time.Minute)
	}
}

func mustGetenv(key string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		log.Fatalf("env %q is required", key)
	}
	return val
}
