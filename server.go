package main

import (
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
	"github.com/urfave/negroni"
)

var (
	mainRouter  *mux.Router
	disableAuth = false // disable token introspection for testing purposes

	tokenInfoEndpoint string
	tokenInfoUser     string
	tokenInfoPassword string

	//useSSL     bool
	newSession *session.Session
	svc        *s3.S3
	//err        error
	//filePath   string
	maxMemory int64

	mysqlHost     string
	mysqlDatabase string
	mysqlUsername string
	mysqlPassword string
	mysqlDSN      string // Data Source Name

	s3rootFolder string
	s3bucket     string

	version = "[[VERSION]]"
)

func init() {
	fmt.Println("hello world")
	tokenInfoEndpoint = os.Getenv("tokenInfoEndpoint")
	tokenInfoUser = os.Getenv("tokenInfoUser")
	tokenInfoPassword = os.Getenv("tokenInfoPassword")

}

func configS3() {

	var s3Endpoint string
	var s3accessKeyID string
	var s3secretAccessKey string

	//flag.StringVar(&s3Endpoint, "s3Endpoint", "", "")
	//flag.StringVar(&s3accessKeyID, "s3accessKeyID", "", "")
	//flag.StringVar(&s3secretAccessKey, "s3secretAccessKey", "", "")
	s3Endpoint = os.Getenv("s3Endpoint")
	s3accessKeyID = os.Getenv("s3accessKeyID")
	s3secretAccessKey = os.Getenv("s3secretAccessKey")
	s3bucket = os.Getenv("s3bucket")
	s3rootFolder = os.Getenv("s3rootFolder")
	log.Printf("s3Endpoint: %s", s3Endpoint)
	log.Printf("s3accessKeyID: %s", s3accessKeyID)
	log.Printf("s3bucket: %s", s3bucket)
	log.Printf("s3rootFolder: %s", s3rootFolder)

	//flag.Parse()

	// flag library makes problems when using the test library
	//see https://github.com/golang/go/issues/33774

	if s3Endpoint == "" {
		log.Fatalf("s3Endpoint not defined")
		return
	}

	if s3bucket == "" {
		log.Fatalf("s3bucket not defined")
		return
	}

	region := "us-west-2"
	//region := "us-east-1" // minio default
	disableSSL := false
	s3FPS := true
	maxMemory = 32 << 20 // 32Mb

	log.Printf("s3Endpoint: %s", s3Endpoint)

	// Initialize s3
	s3Config := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(s3accessKeyID, s3secretAccessKey, ""),
		Endpoint:         aws.String(s3Endpoint),
		Region:           aws.String(region),
		DisableSSL:       aws.Bool(disableSSL),
		S3ForcePathStyle: aws.Bool(s3FPS),
	}
	newSession = session.New(s3Config)
	svc = s3.New(newSession)

}

func createRouter() {

	disableAuth = getEnvBool("NO_AUTH", false)

	configS3()

	mainRouter = mux.NewRouter()
	r := mainRouter

	//r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	//	fmt.Fprintf(w, `{"id": "",\n"available_resources":["api/v1/"],\n"version":"%s"}`, version)
	//})
	r.HandleFunc("/", rootHandler)

	log.Println("SAGE object store (node data)")
	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		fmt.Fprint(w, `{"id": "SAGE object store (node data)",\n"available_resources":["data"]}`)
	})

	// GET /data/
	api.Handle("/data/{jobID}/{taskID}/{nodeID}/{timestampAndFilename}", negroni.New(
		negroni.HandlerFunc(authMW),
		negroni.Wrap(http.HandlerFunc(getFileRequest)),
	)).Methods(http.MethodGet)

	// http.Handle("/metrics", promhttp.Handler())
	r.Handle("/metrics", negroni.New(
		negroni.HandlerFunc(authMW),
		negroni.Wrap(promhttp.Handler()),
	)).Methods(http.MethodGet)

	// match everything else...
	api.NewRoute().PathPrefix("/").HandlerFunc(defaultHandler)

	log.Fatalln(http.ListenAndServe(":80", r))

}

func main() {

	createRouter()
}
