// Copyright (c) 2021 Contributors to the Eclipse Foundation
//
// See the NOTICE file(s) distributed with this work for additional
// information regarding copyright ownership.
//
// This program and the accompanying materials are made available under the
// terms of the Eclipse Public License 2.0 which is available at
// http://www.eclipse.org/legal/epl-2.0
//
// SPDX-License-Identifier: EPL-2.0

package uploaders

import (
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"testing"
)

const testBody = "just a test"

var (
	testFile    string
	testHeaders = map[string]string{"name-a": "value-a", "name-b": "value-b"}

	handler *TestHTTPHandler
	server  *http.Server
)

type TestHTTPHandler struct {
	method  string
	body    []byte
	err     error
	headers http.Header
}

func (h *TestHTTPHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	h.method = req.Method
	h.headers = req.Header
	if req.Body != nil {
		h.body, h.err = ioutil.ReadAll(req.Body)
		req.Body.Close()
	}
}

func (h *TestHTTPHandler) reset() {
	h.method = ""
	h.body = nil
	h.err = nil
	h.headers = nil
}

func TestMain(m *testing.M) {
	setUp()
	code := m.Run()
	tearDown()
	os.Exit(code)
}

func setUp() {
	f, err := os.CreateTemp("./", "test")
	if err != nil {
		log.Fatalln(err)
	}

	defer f.Close()

	testFile = f.Name()
	f.WriteString(testBody)

	handler = &TestHTTPHandler{}
	mux := http.ServeMux{}
	mux.Handle("/up", handler)

	server = &http.Server{
		Addr:    "localhost:1234",
		Handler: &mux,
	}

	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		log.Fatalln(err)
	}

	go func() {
		if err := server.Serve(ln); err != nil {
			log.Println(err)
		}
	}()
}

func tearDown() {
	report(server.Close())

	report(os.Remove(testFile))
}

func TestHTTPUploadDefaultWithoutChecksum(t *testing.T) {
	testHTTPUploadMethod(t, "", false)
}

func TestHTTPUploadDefaultWithChecksum(t *testing.T) {
	testHTTPUploadMethod(t, "", true)
}

func TestHTTPUploadPUTWithoutChecksum(t *testing.T) {
	testHTTPUploadMethod(t, "PUT", false)
}

func TestHTTPUploadPUTWithChecksum(t *testing.T) {
	testHTTPUploadMethod(t, "PUT", true)
}

func TestHTTPUploadPOSTWithoutChecksum(t *testing.T) {
	testHTTPUploadMethod(t, "POST", false)
}

func TestHTTPUploadPOSTWithChecksum(t *testing.T) {
	testHTTPUploadMethod(t, "POST", true)
}

func TestNewHttpUploaderErrors(t *testing.T) {
	options := map[string]string{}

	u, err := NewHTTPUploader(options)
	assertNil(t, u)
	assertError(t, err)

	options[URLProp] = "http://localhost/up"
	options[MethodProp] = "GET"

	u, err = NewHTTPUploader(options)
	assertNil(t, u)
	assertError(t, err)
}

func TestHTTPUploadPortFailure(t *testing.T) {
	testHTTPUploadFailureAddr(t, "http://localhost:5678/up")
}

func TestHTTPUploadAliasFailure(t *testing.T) {
	testHTTPUploadFailureAddr(t, "http://localhost:1234/wrong")
}

func testHTTPUploadFailureAddr(t *testing.T, addr string) {
	f, err := os.Open(testFile)
	assertNoError(t, err)

	defer f.Close()
	defer handler.reset()

	options := map[string]string{
		URLProp: addr,
	}

	u, err := NewHTTPUploader(options)
	assertNoError(t, err)

	err = u.UploadFile(f, false, nil)
	assertError(t, err)
}

func testHTTPUploadMethod(t *testing.T, method string, useChecksum bool) {
	f, err := os.Open(testFile)
	assertNoError(t, err)

	defer f.Close()
	defer handler.reset()

	options := map[string]string{
		URLProp: "http://localhost:1234/up",
	}

	if method != "" {
		options[MethodProp] = method
	} else {
		method = "PUT"
	}

	addAll(options, prefixKeys(testHeaders, HeadersPrefix))

	u, err := NewHTTPUploader(options)
	assertNoError(t, err)

	md5 := getChecksum(t, f, useChecksum)
	err = u.UploadFile(f, true, nil)
	assertNoError(t, err)

	assertNoError(t, handler.err)
	assertStringsSame(t, "request method", method, handler.method)
	assertStringsSame(t, "request body", testBody, string(handler.body))

	if useChecksum {
		assertStringsSame(t, "content md5", *md5, handler.headers.Get(ContentMD5))
	}

	for k, v := range testHeaders {
		h := handler.headers.Get(k)

		assertStringsSame(t, "header "+k, v, h)
	}
}

func getChecksum(t *testing.T, f *os.File, useChecksum bool) *string {
	if useChecksum {
		md5, err := ComputeMD5(f, true)
		assertNoError(t, err)

		return &md5
	}

	return nil
}

func TestExtractDictionary(t *testing.T) {
	info := map[string]string{"name": "John Doe", "age": "37", "addr": "under the bridge"}
	headers := map[string]string{"content-type": "application/x-binary", "content-length": "42"}

	const infoPrefix = "info."
	const headersPrefix = "header."

	options := make(map[string]string)
	addAll(options, prefixKeys(info, infoPrefix))
	addAll(options, prefixKeys(headers, headersPrefix))

	checkExtracted(t, options, infoPrefix, info)
	checkExtracted(t, options, headersPrefix, headers)
}

func report(err error) {
	if err != nil {
		log.Println(err)
	}
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Error(err)
	}
}

func assertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("error expected")
	}
}

func assertNil(t *testing.T, obj interface{}) {
	t.Helper()
	if obj != nil {
		t.Fatal("nil expected")
	}
}

func assertEquals(t *testing.T, name string, expected int64, actual int64) {
	t.Helper()
	if expected != actual {
		t.Fatalf("%v does not match - expected '%v', but was '%v'", name, expected, actual)
	}
}

func assertStringsSame(t *testing.T, name string, expected string, actual string) {
	t.Helper()
	if expected != actual {
		t.Fatalf("%s does not match - expected '%s', but was '%s'", name, expected, actual)
	}
}

func checkExtracted(t *testing.T, options map[string]string, prefix string, expected map[string]string) {
	extracted := ExtractDictionary(options, prefix)
	if !reflect.DeepEqual(expected, extracted) {
		t.Fatalf("Extracted dictionary not equal to original for prefix '%s' - expected %v, but was %v", prefix, expected, extracted)
	}
}

func addAll(m map[string]string, c map[string]string) {
	for k, v := range c {
		m[k] = v
	}
}

func prefixKeys(m map[string]string, prefix string) map[string]string {
	r := make(map[string]string)

	for k, v := range m {
		nk := prefix + k

		r[nk] = v
	}

	return r
}

func partialCopy(m map[string]string, omit string) map[string]string {
	c := map[string]string{}

	for k, v := range m {
		if k != omit {
			c[k] = v
		}
	}

	return c
}

func assertFailsWith(t *testing.T, u interface{}, err error, msg string) {
	t.Helper()

	assertNil(t, u)

	if err == nil {
		t.Fatalf("error '%s' expected", msg)
	}

	if err.Error() != msg {
		t.Fatalf("expected error '%s', but was '%v'", msg, err)
	}
}
