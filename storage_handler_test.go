package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
)

var testMethods = []string{http.MethodGet, http.MethodHead}

func TestHandlerGetUnauthorized(t *testing.T) {
	handler := &StorageHandler{
		Storage:       &mockStorage{},
		Authenticator: &mockAuthenticator{false},
	}
	resp := getResponse(t, handler, http.MethodGet, randomURL())
	assertStatusCode(t, resp, http.StatusUnauthorized)
	assertReadContent(t, resp, []byte(`{
  "error": "not authorized"
}
`))
}

func TestHandlerGetAuthorized(t *testing.T) {
	handler := &StorageHandler{
		Storage:       &mockStorage{},
		Authenticator: &mockAuthenticator{true},
	}
	resp := getResponse(t, handler, http.MethodGet, randomURL())
	assertStatusCode(t, resp, http.StatusTemporaryRedirect)
	// TODO(sean) should we check anything about the URL or is that too much implementation detail?
}

func TestHandlerHeadIgnoreAuth(t *testing.T) {
	for _, auth := range []bool{true, false} {
		url := randomURL()
		handler := &StorageHandler{
			Storage: &mockStorage{
				files: map[string][]byte{
					url: randomContent(),
				},
			},
			Authenticator: &mockAuthenticator{auth},
		}
		resp := getResponse(t, handler, http.MethodHead, url)
		assertStatusCode(t, resp, http.StatusOK)
		assertReadContent(t, resp, []byte(``))
	}
}

func TestHandlerHeadNotFound(t *testing.T) {
	handler := &StorageHandler{
		Storage:       &mockStorage{},
		Authenticator: &mockAuthenticator{true},
	}
	resp := getResponse(t, handler, http.MethodHead, randomURL())
	assertStatusCode(t, resp, http.StatusNotFound)
}

func TestHandlerValidURL(t *testing.T) {
	handler := &StorageHandler{
		Storage: &mockStorage{
			files: map[string][]byte{
				"job/task/node/1643842551600000001-sample.jpg":                   randomContent(),
				"job/task/node/1643842551600000002-sample.jpg":                   randomContent(),
				"job/task/node/1643842551600000003-can-have-multiple-dashes.jpg": randomContent(),
			},
		},
		Authenticator: &mockAuthenticator{true},
	}

	testcases := map[string]struct {
		URL   string
		Valid bool
	}{
		"Valid1":             {"job/task/node/1643842551600000001-sample.jpg", true},
		"Valid2":             {"job/task/node/1643842551600000002-sample.jpg", true},
		"ValidMultiDash":     {"job/task/node/1643842551600000003-can-have-multiple-dashes.jpg", true},
		"NoTimestamp":        {"job/task/node/sample.jpg", false},
		"TooFewSlashes":      {"task/node/1643842551688168762-sample.jpg", false},
		"TooManySlashes":     {"extra/job/task/node/1643842551688168762-sample.jpg", false},
		"BadTimestampLength": {"job/task/node/16438425516881687620-sample.jpg", false},
		"BadTimestampChars":  {"job/task/node/164384X551688168762-sample.jpg", false},
		"EmptyJob":           {"/task/node/164384X551688168762-sample.jpg", false},
		"EmptyTask":          {"job//node/164384X551688168762-sample.jpg", false},
		"EmptyNode":          {"job/task//164384X551688168762-sample.jpg", false},
		"EmptyFilename":      {"job/task/node/", false},
	}

	for name, tc := range testcases {
		for _, method := range testMethods {
			t.Run(name+"/"+method, func(t *testing.T) {
				resp := getResponse(t, handler, method, tc.URL)
				switch {
				case tc.Valid && method == http.MethodGet:
					assertStatusCode(t, resp, http.StatusTemporaryRedirect)
				case tc.Valid && method == http.MethodHead:
					assertStatusCode(t, resp, http.StatusOK)
				default:
					assertStatusCode(t, resp, http.StatusBadRequest)
				}
			})
		}
	}
}

func TestHandlerGetContentDisposition(t *testing.T) {
	testcases := []struct {
		URL      string
		Filename string
	}{
		{"job/task/node/1643842551600000000-sample.jpg", "1643842551600000000-sample.jpg"},
		{"job/task/node/1643842551600000001-audio.mp3", "1643842551600000001-audio.mp3"},
		{"job/task/node/1643842551600000002-important.txt", "1643842551600000002-important.txt"},
		{"job/task/node/1643842551600000003-thermal.rgb", "1643842551600000003-thermal.rgb"},
	}

	files := make(map[string][]byte)
	for _, tc := range testcases {
		files[tc.URL] = randomContent()
	}
	handler := &StorageHandler{
		Storage:       &mockStorage{files: files},
		Authenticator: &mockAuthenticator{true},
	}

	for _, tc := range testcases {
		resp := getResponse(t, handler, http.MethodGet, tc.URL)
		assertStatusCode(t, resp, http.StatusTemporaryRedirect)
		assertContentDisposition(t, resp, fmt.Sprintf("attachment; filename=%s", tc.Filename))
	}
}

func TestHandlerCORSHeaders(t *testing.T) {
	handler := &StorageHandler{
		Storage:       &mockStorage{},
		Authenticator: &mockAuthenticator{true},
	}

	for _, method := range testMethods {
		resp := getResponse(t, handler, method, randomURL())

		allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
		if allowOrigin != "*" {
			t.Fatalf("Access-Control-Allow-Origin must be *. got %q", allowOrigin)
		}

		// TODO(sean) check other expected headers
		// methods := resp.Header.Values("Access-Control-Allow-Methods")
		// sort.Strings(methods)
		// if strings.Join(methods, ",") != "GET,HEAD,OPTIONS" {
		// 	t.Fatalf("allow methods must be GET, HEAD and OPTIONS")
		// }
	}
}

// mockS3Client provides a fixed set of content using an in-memory map of URLs to data
type mockStorage struct {
	files map[string][]byte
}

func (s *mockStorage) GetObjectInfo(ctx context.Context, key string) (*s3.HeadObjectOutput, error) {
	if s.files == nil {
		return nil, awserr.New(s3.ErrCodeNoSuchKey, "", nil)
	}
	content, ok := s.files[key]
	if !ok {
		return nil, awserr.New(s3.ErrCodeNoSuchKey, "", nil)
	}
	return &s3.HeadObjectOutput{
		ContentLength: aws.Int64(int64(len(content))),
	}, nil
}

func (s *mockStorage) GetObjectPresignedURL(ctx context.Context, key string) (string, error) {
	return fmt.Sprintf("https://real-storage-host/%s", key), nil
}

// mockAuthenticator provides a simple "allow all" or "reject all" policy for testing
type mockAuthenticator struct {
	authorized bool
}

func (a *mockAuthenticator) Authorized(f *StorageFile, username, password string, hasAuth bool) bool {
	return a.authorized
}

func getResponse(t *testing.T, h http.Handler, method string, url string) *http.Response {
	r, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("error when creating request: %s", err.Error())
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Result()
}

func assertStatusCode(t *testing.T, resp *http.Response, status int) {
	if resp.StatusCode != status {
		t.Fatalf("incorrect status code. got: %d want: %d", resp.StatusCode, status)
	}
}

func assertContentDisposition(t *testing.T, resp *http.Response, expect string) {
	s := resp.Header.Get("Content-Disposition")
	if s != expect {
		t.Fatalf("incorrect content disposition. got: %s. want: %s", s, expect)
	}
}

func assertReadContent(t *testing.T, resp *http.Response, content []byte) {
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("error when reading body: %s", err.Error())
	}
	if !bytes.Equal(b, content) {
		t.Fatalf("content does not match. got: %v want: %v", b, content)
	}
}

func randomString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

func randomURL() string {
	ts := time.Unix(0, rand.Int63()).UnixNano()
	return fmt.Sprintf("%s/%s/%s/%d-%s", randomString(11), randomString(13), randomString(16), ts, randomString(23))
}

func randomContent() []byte {
	length := rand.Intn(1234) + 33
	b := make([]byte, length)
	for i := range b {
		b[i] = byte(rand.Intn(length))
	}
	return b
}
