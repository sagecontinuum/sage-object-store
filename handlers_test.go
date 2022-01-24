package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
)

func TestRootHandler(t *testing.T) {

	req, err := http.NewRequest("GET", "/", nil)

	if err != nil {
		t.Fatalf("failed: %s", err.Error())
	}

	rr := httptest.NewRecorder()

	http.HandlerFunc(rootHandler).ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	result := RootResponse{}

	bs, _ := rr.Body.ReadBytes('@')
	t.Logf("bs: %s", bs)
	json.Unmarshal(bs, &result)

	// expect
	// {
	//	"id": "SAGE object store (node data)",
	//	"available_resources": [
	//	  "api/v1/"
	//	],
	//	"version": "[[VERSION]]"
	//  }%

	t.Logf("got: %v", result)
	if len(result.ID) == 0 {
		t.Fatal("response has no ID")
	}
	//t.Fatal("---")
}

func TestHeadFileRequest(t *testing.T) {

	configS3(true)
	r := createRouter()

	req, err := http.NewRequest("HEAD", "/api/v1/data/j/t/n/0001-sample.jpg", nil)

	if err != nil {
		t.Fatalf("failed: %s", err.Error())
	}

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	//http.HandlerFunc(headFileRequest).ServeHTTP(rr, req)
	t.Logf("body: %s", rr.Body.String())
	status := rr.Code
	if status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
	t.Logf("body: %s", rr.Body.String())

	result := s3.HeadObjectOutput{}

	bs, _ := rr.Body.ReadBytes('@')
	t.Logf("bs: %s", bs)
	json.Unmarshal(bs, &result)

	if result.ContentLanguage == nil {
		t.Fatal("ContentLanguage missing")
	}
	if *(result.ContentLanguage) != "klingon" {
		t.Fatal("ContentLanguage wrong")
	}
}

func TestGetFileRequest(t *testing.T) {

	var mytests = []struct {
		auth         bool
		url          string
		expectStatus int
	}{
		{
			auth:         false,
			url:          "/api/v1/data/j/t/abc/" + fmt.Sprintf("%d", time.Now().UnixNano()) + "-sample.jpg",
			expectStatus: http.StatusOK,
		},
		{
			auth: false,
			// three years ago, abc was not commissioned
			url:          "/api/v1/data/j/t/abc/" + fmt.Sprintf("%d", time.Now().AddDate(-3, 0, 0).UnixNano()) + "-sample.jpg",
			expectStatus: http.StatusUnauthorized,
		},
		{
			auth:         false,
			url:          "/api/v1/data/j/task_bottom/abc/0001-sample.jpg",
			expectStatus: http.StatusUnauthorized,
		},
		{
			auth:         true,
			url:          "/api/v1/data/j/task_bottom/abc/0001-sample.jpg",
			expectStatus: http.StatusOK,
		},
	}

	os.Setenv("policyRestrictedNodes", "abc,bca")
	os.Setenv("policyRestrictedTaskSubstrings", "bottom,street")
	os.Setenv("policyRestrictedUsername", "user")
	os.Setenv("policyRestrictedPassword", "secret")

	configS3(true)
	r := createRouter()

	for _, test := range mytests {
		_ = test
		t.Logf("url: %s", test.url)
		req, err := http.NewRequest("GET", test.url, nil)

		if err != nil {
			t.Fatalf("failed: %s", err.Error())
		}

		if test.auth {
			req.SetBasicAuth("user", "secret")
		}

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		status := rr.Code
		if status != test.expectStatus {
			t.Errorf("handler returned wrong status code: got %v want %v", status, test.expectStatus)
		}
		t.Logf("body: %s", rr.Body.String())

	}

	_ = mytests
}

func TestMiddleware(t *testing.T) {

	req, err := http.NewRequest("GET", "/", nil)

	if err != nil {
		t.Fatalf("failed: %s", err.Error())
	}

	rr := httptest.NewRecorder()

	r := createRouter()
	r.ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusOK {
		t.Fatalf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	acao := rr.Header().Get("Access-Control-Allow-Origin")
	if acao != "*" {
		t.Fatalf("Access-Control-Allow-Origin header wrong, got \"%s\"", acao)
	}

	acam := rr.Header().Values("Access-Control-Allow-Methods")
	if len(acam) == 0 {
		t.Fatalf("Access-Control-Allow-Origin header empty")
	}
	if strings.Join(acam, ",") != "GET,OPTIONS" {
		t.Fatalf("Access-Control-Allow-Methods header wrong, got \"%s\"", strings.Join(acam, ","))
	}
}
