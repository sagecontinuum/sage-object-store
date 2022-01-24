package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	//mainRouter  *mux.Router
	disableAuth = false // disable token introspection for testing purposes

	tokenInfoEndpoint string
	tokenInfoUser     string
	tokenInfoPassword string

	//useSSL     bool
	//newSession *session.Session
	//svc        *s3.S3
	//myS3 S3Client
	svc s3iface.S3API

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

	policyRestrictedNodes          map[string]bool
	policyRestrictedTaskSubstrings []string
	policyRestrictedUsername       string
	policyRestrictedPassword       string

	nodeCommissionDates     map[string]time.Time
	nodeCommissionDatesLock sync.RWMutex
)

func GetNodeCommissionDate(nodeID string) (cdate time.Time, ok bool) {
	nodeCommissionDatesLock.RLock()
	defer nodeCommissionDatesLock.RUnlock()

	log.Printf("HAVE: -----------\n")
	for key, _ := range nodeCommissionDates {
		log.Printf("HAVE: %s\n", key)
	}
	cdate, ok = nodeCommissionDates[nodeID]
	if !ok {
		return
	}
	return
}

type ProductionNode struct {
	NodeID         string `json:"node_id"`
	CommissionDate string `json:"commission_date"`
}

type mockS3Client struct {
	s3iface.S3API
}

func (m *mockS3Client) HeadObjectWithContext(ctx context.Context, hoi *s3.HeadObjectInput, opts ...request.Option) (*s3.HeadObjectOutput, error) {
	_ = ctx
	klingon := "klingon"
	return &s3.HeadObjectOutput{ContentLanguage: &klingon}, nil
}

func (m *mockS3Client) GetObjectWithContext(context.Context, *s3.GetObjectInput, ...request.Option) (*s3.GetObjectOutput, error) {

	result := &s3.GetObjectOutput{}

	file_content := "I am fake file content"
	result.Body = io.NopCloser(strings.NewReader(file_content))
	file_len := int64(len(file_content))
	result.ContentLength = &file_len
	return result, nil
}

func init() {
	fmt.Println("hello world")
	tokenInfoEndpoint = os.Getenv("tokenInfoEndpoint")
	tokenInfoUser = os.Getenv("tokenInfoUser")
	tokenInfoPassword = os.Getenv("tokenInfoPassword")

}

func configS3(useMockS3 bool) {

	var s3Endpoint string
	var s3accessKeyID string
	var s3secretAccessKey string

	policyRestrictedNodes_str := os.Getenv("policyRestrictedNodes")
	policyRestrictedTaskSubstrings_str := os.Getenv("policyRestrictedTaskSubstrings")

	policyRestrictedNodes = make(map[string]bool)
	policyRestrictedNodes_array := strings.Split(policyRestrictedNodes_str, ",")
	for _, elem := range policyRestrictedNodes_array {
		policyRestrictedNodes[strings.ToLower(elem)] = true
		fmt.Println(elem + " add to policyRestrictedNodes")
	}
	policyRestrictedTaskSubstrings = strings.Split(policyRestrictedTaskSubstrings_str, ",")
	fmt.Printf("policyRestrictedTaskSubstrings len: %d\n", len(policyRestrictedTaskSubstrings))

	policyRestrictedUsername = os.Getenv("policyRestrictedUsername")
	policyRestrictedPassword = os.Getenv("policyRestrictedPassword")

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

	if !useMockS3 {
		if s3Endpoint == "" {
			log.Fatalf("s3Endpoint not defined")
			return
		}

		if s3bucket == "" {
			log.Fatalf("s3bucket not defined")
			return
		}
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

	if !useMockS3 {
		//var err error
		session, err := session.NewSession(s3Config)
		if err != nil {
			log.Fatal(err.Error())
		}
		svc = s3.New(session)
	} else {
		svc = &mockS3Client{}

	}

}

func createRouter() (r *mux.Router) {

	disableAuth = getEnvBool("NO_AUTH", false)

	r = mux.NewRouter()

	r.HandleFunc("/", rootHandler).Methods(http.MethodGet, http.MethodOptions)

	log.Println("SAGE object store (node data)")

	if os.Getenv("TESTING") == "1" {
		nodeCommissionDates = make(map[string]time.Time)
		nodeCommissionDates["abc"] = time.Now().AddDate(-1, 0, 0) // commissioned one year ago
		nodeCommissionDates["bca"] = time.Now().AddDate(-1, 0, 0) // commissioned one year ago
	}

	//api := r.PathPrefix("/api/v1").Subrouter()

	r.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {

		fmt.Fprint(w, `{"id": "SAGE object store (node data)",\n"available_resources":["data"]}`)
	})

	// GET /data/
	r.Handle("/api/v1/data/{jobID}/{taskID}/{nodeID}/{timestampAndFilename}", http.HandlerFunc(getFileRequest)).Methods(http.MethodGet, http.MethodOptions)

	// HEAD /data
	r.Handle("/api/v1/data/{jobID}/{taskID}/{nodeID}/{timestampAndFilename}", http.HandlerFunc(headFileRequest)).Methods(http.MethodHead)

	// http.Handle("/metrics", promhttp.Handler())
	r.Handle("/metrics", promhttp.Handler()).Methods(http.MethodGet)

	// match everything else...
	//api.NewRoute().PathPrefix("/").HandlerFunc(defaultHandler)

	r.NotFoundHandler = http.HandlerFunc(defaultHandler)

	r.Use(mux.CORSMethodMiddleware(r))

	r.Use(Middleware)
	//r.Use(authMW)

	return

}

func getcommission_dates(productionURL string) (err error) {
	log.Print("running getcommission_dates")

	resp, err := http.Get(productionURL)
	if resp.StatusCode != 200 {
		err = fmt.Errorf("(getcommission_dates) Got resp.StatusCode: %d", resp.StatusCode)
		return
	}
	if err != nil {
		err = fmt.Errorf("(getcommission_dates) Could not retrive url: %s", err.Error())
		return
	}

	nodeList := []ProductionNode{}

	err = json.NewDecoder(resp.Body).Decode(&nodeList)
	if err != nil {
		err = fmt.Errorf("(getcommission_dates) Could not parse json: %s", err.Error())
		return
	}

	nodeCommissionDatesLock.Lock()
	defer nodeCommissionDatesLock.Unlock()

	// overwrite old map
	nodeCommissionDates = make(map[string]time.Time)

	for _, node := range nodeList {

		if node.NodeID == "" {
			continue
		}

		log.Println(node.NodeID)
		if len(node.CommissionDate) == 0 {
			continue
		}
		//log.Println(node.CommissionDate)

		if len(node.CommissionDate) != 10 {
			log.Printf("CommissionDate format wrong: %s\n", node.CommissionDate)
			continue
		}
		var year int
		var month int
		var day int

		year, err = strconv.Atoi(node.CommissionDate[0:4])
		if err != nil {
			return
		}
		month, err = strconv.Atoi(node.CommissionDate[5:7])
		if err != nil {
			return
		}
		day, err = strconv.Atoi(node.CommissionDate[8:10])
		if err != nil {
			return
		}

		log.Printf("extracted: %d %d %d\n", year, month, day)

		nodeCommissionDates[strings.ToLower(node.NodeID)] = time.Date(year, time.Month(month), day, 1, 1, 1, 1, time.UTC)

		//log.Print(node.CommissionDate)
	}

	return
}

func loop_getcommission_dates() {
	productionURL := os.Getenv("productionURL")
	if productionURL == "" {
		log.Print("no productionURL found")
		return
	}
	for {
		err := getcommission_dates(productionURL)
		if err != nil {
			log.Printf("Error: %s", err.Error())
		}
		time.Sleep(10 * time.Minute)
	}
}

func main() {

	//if os.Getenv("TESTING") != "1" {
	useMockS3 := os.Getenv("TESTING") == "1"
	if useMockS3 {
		log.Print("mocking enabled")
	}
	configS3(useMockS3)
	//}

	if os.Getenv("TESTING") != "1" { // see createRouter for testing data
		go loop_getcommission_dates()
	}

	r := createRouter()
	log.Fatalln(http.ListenAndServe(":80", r))
}
