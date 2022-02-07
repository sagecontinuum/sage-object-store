package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/gorilla/mux"
)

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
	content := "I am fake file content"
	result.Body = io.NopCloser(strings.NewReader(content))
	length := int64(len(content))
	result.ContentLength = &length
	return result, nil
}

func makeCommissionDate(year, month, day int) *time.Time {
	t := time.Now().AddDate(year, month, day)
	return &t
}

func getTestRouter() *mux.Router {
	TableAuthenticator := &TableAuthenticator{}

	TableAuthenticator.UpdateConfig(&TableAuthenticatorConfig{
		Username: "user",
		Password: "secret",
		Nodes: map[string]*TableAuthenticatorNode{
			"uncommissioned": {
				Restricted: false,
			},
			"commissioned1Y": {
				Restricted:     false,
				CommissionDate: makeCommissionDate(-1, 0, 0),
			},
			"commissioned3Y": {
				Restricted:     false,
				CommissionDate: makeCommissionDate(-3, 0, 0),
			},
			"restrictedNode1": {
				Restricted:     true,
				CommissionDate: makeCommissionDate(-1, 0, 0),
			},
			"restrictedNode2": {
				Restricted:     true,
				CommissionDate: makeCommissionDate(-1, 0, 0),
			},
		},
		RestrictedTasksSubstrings: []string{
			"imagesampler-bottom",
			"imagesampler-left",
			"imagesampler-right",
			"imagesampler-top",
			"audiosampler",
		},
	})

	handler := &StorageHandler{
		S3API:         &mockS3Client{},
		Authenticator: TableAuthenticator,
	}

	return createRouter(handler)
}

func makePolicyHelperFuncs(router *mux.Router) (func(urls ...string) bool, func(urls ...string) bool) {
	// unauthorized checks if response code is Unauthorized
	unauthorized := func(url string, username, password string, hasAuth bool) bool {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return false
		}
		if hasAuth {
			req.SetBasicAuth(username, password)
		}
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr.Code == http.StatusUnauthorized
	}

	// authorized checks if response code is OK and that content matches mock file
	authorized := func(url string, username, password string, hasAuth bool) bool {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return false
		}
		if hasAuth {
			req.SetBasicAuth(username, password)
		}
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			return false
		}
		// this will be confusing here
		// if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		// 	t.Fatalf("Access-Control-Allow-Origin != *, instead %q", rr.Header().Get("Access-Control-Allow-Origin"))
		// }
		b, err := io.ReadAll(rr.Result().Body)
		if err != nil {
			return false
		}
		expectBody := []byte("I am fake file content")
		if !bytes.Equal(b, expectBody) {
			return false
		}
		if rr.Result().ContentLength != int64(len(expectBody)) {
			return false
		}
		return true
	}

	public := func(urls ...string) bool {
		for _, url := range urls {
			if !authorized(url, "", "", false) {
				return false
			}
			if !authorized(url, "any", "credentials", true) {
				return false
			}
			if !authorized(url, "user", "secret", true) {
				return false
			}
		}
		return true
	}

	private := func(urls ...string) bool {
		for _, url := range urls {
			if !unauthorized(url, "", "", false) {
				return false
			}
			if !unauthorized(url, "wrong", "credentials", true) {
				return false
			}
			if !authorized(url, "user", "secret", true) {
				return false
			}
		}
		return true
	}

	return public, private
}

func TestHeadRequest(t *testing.T) {
	r := getTestRouter()

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

func timestamp(t time.Time) string {
	return fmt.Sprintf("%d", t.UnixNano())
}

func TestGetRequest(t *testing.T) {
	// NOTE there is some overlap with the node policy fuzz test. this test is still required as
	// the fuzz test does not check for restricted tasks
	var mytests = map[string]struct {
		public bool
		url    string
	}{
		"allow": {
			public: true,
			url:    fmt.Sprintf("/api/v1/data/sage/safe-task/commissioned1Y/%s-sample.jpg", timestamp(time.Now())),
		},
		"allowPast1": {
			public: true,
			url:    fmt.Sprintf("/api/v1/data/sage/safe-task/commissioned1Y/%s-sample.jpg", timestamp(time.Now().AddDate(0, -6, 0))),
		},
		"allowPast2": {
			public: true,
			url:    fmt.Sprintf("/api/v1/data/sage/safe-task/commissioned3Y/%s-sample.jpg", timestamp(time.Now().AddDate(-2, 0, 0))),
		},
		"allowFuture": {
			public: true,
			url:    fmt.Sprintf("/api/v1/data/sage/safe-task/commissioned1Y/%s-sample.jpg", timestamp(time.Now().AddDate(0, 6, 0))),
		},
		"restrictNode1": {
			public: false,
			url:    fmt.Sprintf("/api/v1/data/sage/safe-task/restrictedNode1/%s-sample.jpg", timestamp(time.Now())),
		},
		"restrictNode2": {
			public: false,
			url:    fmt.Sprintf("/api/v1/data/sage/safe-task/restrictedNode2/%s-sample.jpg", timestamp(time.Now())),
		},
		"restrictTask": {
			public: false,
			url:    fmt.Sprintf("/api/v1/data/sage/imagesampler-bottom/commissioned1Y/%s-sample.jpg", timestamp(time.Now())),
		},
		"restrictPast1": {
			public: false,
			url:    fmt.Sprintf("/api/v1/data/sage/safe-task/commissioned1Y/%s-sample.jpg", timestamp(time.Now().AddDate(-1, 0, -1))),
		},
		"restrictPast2": {
			public: false,
			url:    fmt.Sprintf("/api/v1/data/sage/safe-task/commissioned3Y/%s-sample.jpg", timestamp(time.Now().AddDate(-3, 0, -1))),
		},
		"restrictUncommissioned": {
			public: false,
			url:    fmt.Sprintf("/api/v1/data/sage/safe-task/uncommissioned/%s-sample.jpg", timestamp(time.Now())),
		},
	}

	router := getTestRouter()
	public, private := makePolicyHelperFuncs(router)

	for name, test := range mytests {
		t.Run(name, func(t *testing.T) {
			if test.public {
				if !public(test.url) {
					t.Fatalf("test case %q should have been public", test.url)
				}
			} else {
				if !private(test.url) {
					t.Fatalf("test case %q should have been private", test.url)
				}
			}
		})
	}
}

func randomNodeID() string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	s := make([]rune, 16)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

func generateRandomNodeList(n int) map[string]*TableAuthenticatorNode {
	nodes := make(map[string]*TableAuthenticatorNode)
	for i := 0; i < n; i++ {
		// generate a random commission date in the past
		cdate := time.Now().AddDate(0, 0, -rand.Intn(1000))

		nodes[randomNodeID()] = &TableAuthenticatorNode{
			Restricted:     rand.Intn(2) == 0,
			CommissionDate: &cdate,
		}
	}
	return nodes
}

func TestNodePolicyFuzz(t *testing.T) {
	nodes := generateRandomNodeList(1000)

	TableAuthenticator := &TableAuthenticator{}
	TableAuthenticator.UpdateConfig(&TableAuthenticatorConfig{
		Username:                  "user",
		Password:                  "secret",
		Nodes:                     nodes,
		RestrictedTasksSubstrings: []string{},
	})

	handler := &StorageHandler{
		S3API:         &mockS3Client{},
		Authenticator: TableAuthenticator,
	}

	router := createRouter(handler)
	public, private := makePolicyHelperFuncs(router)

	// check policies against all nodes
	for nodeID, node := range nodes {
		urlAt := func(t time.Time) string {
			return fmt.Sprintf("/api/v1/data/sage/safe-task/%s/%s-sample.jpg", nodeID, timestamp(t))
		}

		switch {
		case node.Restricted:
			if !private(urlAt(time.Now()),
				urlAt(time.Now().AddDate(0, 0, -2000)),
				urlAt(time.Now().AddDate(1, 0, 0))) {
				t.Fatalf("restricted node must be private regardless of commissioning date")
			}
		case !node.Restricted && node.CommissionDate != nil:
			if !public(urlAt(*node.CommissionDate)) {
				t.Fatalf("nonrestricted node with commissioning date must be public on commissioning date")
			}

			if !public(urlAt(node.CommissionDate.AddDate(1, 0, 0))) {
				t.Fatalf("nonrestricted node with commissioning date must be public after commissioning date")
			}

			if !private(urlAt(node.CommissionDate.AddDate(0, 0, -1)),
				urlAt(node.CommissionDate.AddDate(0, -1, 0)),
				urlAt(node.CommissionDate.AddDate(-1, 0, 0))) {
				t.Fatalf("nonrestricted node with commissioning date must be private before commissioning date")
			}
		case !node.Restricted && node.CommissionDate == nil:
			if !private(urlAt(time.Now())) {
				t.Fatalf("nonrestricted node without commissioning date must be private")
			}
		}
	}
}

// func TestMiddleware(t *testing.T) {

// 	req, err := http.NewRequest("GET", "/", nil)

// 	if err != nil {
// 		t.Fatalf("failed: %s", err.Error())
// 	}

// 	rr := httptest.NewRecorder()

// 	r := createRouter()
// 	r.ServeHTTP(rr, req)
// 	if status := rr.Code; status != http.StatusOK {
// 		t.Fatalf("handler returned wrong status code: got %v want %v",
// 			status, http.StatusOK)
// 	}

// 	acao := rr.Header().Get("Access-Control-Allow-Origin")
// 	if acao != "*" {
// 		t.Fatalf("Access-Control-Allow-Origin header wrong, got \"%s\"", acao)
// 	}

// 	acam := rr.Header().Values("Access-Control-Allow-Methods")
// 	if len(acam) == 0 {
// 		t.Fatalf("Access-Control-Allow-Origin header empty")
// 	}
// 	if strings.Join(acam, ",") != "GET,OPTIONS" {
// 		t.Fatalf("Access-Control-Allow-Methods header wrong, got \"%s\"", strings.Join(acam, ","))
// 	}
// }
