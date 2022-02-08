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

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

func TestHandlerHeadUnauthorized(t *testing.T) {
	handler := &StorageHandler{
		S3API:         &mockS3Client{},
		Authenticator: &mockAuthenticator{false},
	}
	resp := getResponse(t, handler, http.MethodHead, randomURL())
	assertStatusCode(t, resp, http.StatusUnauthorized)
}

func TestHandlerHeadNotFound(t *testing.T) {
	handler := &StorageHandler{
		S3API:         &mockS3Client{},
		Authenticator: &mockAuthenticator{true},
	}
	resp := getResponse(t, handler, http.MethodHead, randomURL())
	assertStatusCode(t, resp, http.StatusNotFound)
}

func TestHandlerHeadOK(t *testing.T) {
	content := randomContent()
	url := randomURL()
	handler := &StorageHandler{
		S3API: &mockS3Client{
			files: map[string][]byte{url: content},
		},
		Authenticator: &mockAuthenticator{true},
	}
	resp := getResponse(t, handler, http.MethodHead, url)
	assertStatusCode(t, resp, http.StatusOK)
	assertContentLength(t, resp, len(content))
}

func TestHandlerValidURL(t *testing.T) {
	handler := &StorageHandler{
		S3API: &mockS3Client{
			files: map[string][]byte{
				"job/task/node/1643842551600000001-sample.jpg": []byte("data1"),
				"job/task/node/1643842551600000002-sample.jpg": []byte("data2"),
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
		"TooFewSlashes":      {"task/node/1643842551688168762-sample.jpg", false},
		"TooManySlashes":     {"extra/job/task/node/1643842551688168762-sample.jpg", false},
		"EmptyJob":           {"/task/node/164384X551688168762-sample.jpg", false},
		"EmptyTask":          {"job//node/164384X551688168762-sample.jpg", false},
		"EmptyNode":          {"job/task//164384X551688168762-sample.jpg", false},
		"EmptyFilename":      {"job/task/node/", false},
		"BadTimestampLength": {"sage/task/node/16438425516881687620-sample.jpg", false},
		"BadTimestampChars":  {"sage/task/node/164384X551688168762-sample.jpg", false},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				resp := getResponse(t, handler, method, tc.URL)
				if tc.Valid {
					assertStatusCode(t, resp, http.StatusOK)
				} else {
					assertStatusCode(t, resp, http.StatusInternalServerError)
				}
			}
		})
	}
}

func TestHandlerGetUnauthorized(t *testing.T) {
	handler := &StorageHandler{
		S3API:         &mockS3Client{},
		Authenticator: &mockAuthenticator{false},
	}
	resp := getResponse(t, handler, http.MethodGet, randomURL())
	assertStatusCode(t, resp, http.StatusUnauthorized)
	assertReadContent(t, resp, []byte(`{
  "error": "not authorized"
}
`))
}

func TestHandlerGetNotFound(t *testing.T) {
	handler := &StorageHandler{
		S3API:         &mockS3Client{},
		Authenticator: &mockAuthenticator{true},
	}
	resp := getResponse(t, handler, http.MethodGet, randomURL())
	assertStatusCode(t, resp, http.StatusNotFound)
}

func TestHandlerGetOK(t *testing.T) {
	content := randomContent()
	url := randomURL()
	handler := &StorageHandler{
		S3API: &mockS3Client{
			files: map[string][]byte{url: content},
		},
		Authenticator: &mockAuthenticator{true},
	}
	resp := getResponse(t, handler, http.MethodGet, url)
	assertStatusCode(t, resp, http.StatusOK)
	assertContentLength(t, resp, len(content))
	assertReadContent(t, resp, content)
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
		S3API: &mockS3Client{
			files: files,
		},
		Authenticator: &mockAuthenticator{true},
	}

	for _, tc := range testcases {
		resp := getResponse(t, handler, http.MethodGet, tc.URL)
		assertStatusCode(t, resp, http.StatusOK)
		assertContentDisposition(t, resp, fmt.Sprintf("attachment; filename=%s", tc.Filename))
	}
}

func TestHandlerCORSHeaders(t *testing.T) {
	handler := &StorageHandler{
		S3API:         &mockS3Client{},
		Authenticator: &mockAuthenticator{true},
	}

	methods := []string{http.MethodGet, http.MethodHead, http.MethodOptions}

	for _, method := range methods {
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
type mockS3Client struct {
	files map[string][]byte
	s3iface.S3API
}

func (m *mockS3Client) HeadObjectWithContext(ctx context.Context, obj *s3.HeadObjectInput, options ...request.Option) (*s3.HeadObjectOutput, error) {
	if obj.Key == nil {
		return nil, fmt.Errorf("no key provided")
	}
	content, ok := m.files[*obj.Key]
	if !ok {
		return nil, fmt.Errorf(s3.ErrCodeNoSuchKey)
	}
	lang := "klingon"
	length := int64(len(content))
	return &s3.HeadObjectOutput{
		ContentLanguage: &lang,
		ContentLength:   &length,
	}, nil
}

func (m *mockS3Client) GetObjectWithContext(ctx context.Context, obj *s3.GetObjectInput, options ...request.Option) (*s3.GetObjectOutput, error) {
	if obj.Key == nil {
		return nil, fmt.Errorf("no key provided")
	}
	content, ok := m.files[*obj.Key]
	if !ok {
		return nil, fmt.Errorf(s3.ErrCodeNoSuchKey)
	}

	length := int64(len(content))
	return &s3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader(content)),
		ContentLength: &length,
	}, nil
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
		t.Errorf("incorrect status code. got: %d want: %d", resp.StatusCode, status)
	}
}

func assertContentLength(t *testing.T, resp *http.Response, length int) {
	if resp.ContentLength != int64(length) {
		t.Errorf("incorrect content length. got: %d want: %d", resp.StatusCode, length)
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
		t.Errorf("content does not match. got: %v want: %v", b, content)
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
