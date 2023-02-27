// Copyright (c) 2021 Contributors to the Eclipse Foundation
//
// See the NOTICE file(s) distributed with this work for additional
// information regarding copyright ownership.
//
// This program and the accompanying materials are made available under the
// terms of the Eclipse Public License 2.0 which is available at
// https://www.eclipse.org/legal/epl-2.0, or the Apache License, Version 2.0
// which is available at https://www.apache.org/licenses/LICENSE-2.0.
//
// SPDX-License-Identifier: EPL-2.0 OR Apache-2.0

//go:build unit

package uploaders

import (
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"testing"
)

const (
	validCert      = "testdata/valid_cert.pem"
	validKey       = "testdata/valid_key.pem"
	expiredCert    = "testdata/expired_cert.pem"
	expiredKey     = "testdata/expired_key.pem"
	testBody       = "just a test"
	sslCertFileEnv = "SSL_CERT_FILE"
)

var (
	testFile    string
	testHeaders = map[string]string{"name-a": "value-a", "name-b": "value-b"}

	handler      *TestHTTPHandler
	server       *http.Server
	serverSecure *http.Server

	sslCertFile string
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
			log.Printf("failed to start web server: %v", err)
		}
	}()

	serverSecure = &http.Server{
		Addr:    "localhost:2345",
		Handler: &mux,
	}

	lnSecure, err := net.Listen("tcp", serverSecure.Addr)
	if err != nil {
		log.Fatalln(err)
	}
	go func() {
		if err := serverSecure.ServeTLS(lnSecure, validCert, validKey); err != nil && err != http.ErrServerClosed {
			log.Printf("failed to start web server: %v", err)
		}
	}()
}

func tearDown() {
	report(server.Close())
	report(serverSecure.Close())

	report(os.Remove(testFile))
}

func isCertAddedToSystemPool(t *testing.T, certFile string) bool {
	t.Helper()

	certs, err := x509.SystemCertPool()
	if err != nil {
		t.Logf("error getting system certificate pool - %v", err)
		return false
	}
	data, err := ioutil.ReadFile(certFile)
	if err != nil {
		t.Logf("error reading certificate file %s - %v", certFile, err)
		return false
	}
	block, _ := pem.Decode(data)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Logf("error parsing certificate %s - %v", certFile, err)
		return false
	}
	subjects := certs.Subjects()
	for i := 0; i < len(subjects); i++ {
		if reflect.DeepEqual(subjects[i], cert.RawSubject) {
			return true
		}
	}
	return false
}

func setSSLCerts(t *testing.T) {
	t.Helper()

	sslCertFile = os.Getenv(sslCertFileEnv)
	err := os.Setenv(sslCertFileEnv, validCert)
	if err != nil {
		t.Skipf("cannot set %s environment variable", sslCertFileEnv)
	}
	if !isCertAddedToSystemPool(t, validCert) {
		t.Skipf("cannot setup test case by adding certificate %s to system certificate pool", validCert)
	}
}

func unsetSSLCerts(t *testing.T) {
	t.Helper()

	if len(sslCertFile) > 0 {
		os.Setenv(sslCertFileEnv, sslCertFile)
	} else {
		os.Unsetenv(sslCertFileEnv)
	}
}

func TestHTTPUploadDefaultWithoutChecksum(t *testing.T) {
	testHTTPUploadMethod(t, "", false, false, "", "")
}

func TestHTTPSUploadDefaultWithoutChecksum(t *testing.T) {
	setSSLCerts(t)
	defer unsetSSLCerts(t)
	testHTTPUploadMethod(t, "", false, true, "", "")
}

func TestHTTPUploadDefaultWithChecksum(t *testing.T) {
	testHTTPUploadMethod(t, "", true, false, "", "")
}

func TestHTTPSUploadDefaultWithChecksum(t *testing.T) {
	setSSLCerts(t)
	defer unsetSSLCerts(t)
	testHTTPUploadMethod(t, "", true, true, "", "")
}

func TestHTTPSUploadDefaultWithCertAndKey(t *testing.T) {
	testHTTPUploadMethod(t, "", false, true, validCert, validKey)
}

func TestHTTPUploadPUTWithoutChecksum(t *testing.T) {
	testHTTPUploadMethod(t, "PUT", false, false, "", "")
}

func TestHTTPSUploadPUTWithoutChecksum(t *testing.T) {
	setSSLCerts(t)
	defer unsetSSLCerts(t)
	testHTTPUploadMethod(t, "PUT", false, true, "", "")
}

func TestHTTPUploadPUTWithChecksum(t *testing.T) {
	testHTTPUploadMethod(t, "PUT", true, false, "", "")
}

func TestHTTPSUploadPUTWithCertAndKey(t *testing.T) {
	testHTTPUploadMethod(t, "PUT", false, true, validCert, validKey)
}

func TestHTTPSUploadPUTWithChecksum(t *testing.T) {
	setSSLCerts(t)
	defer unsetSSLCerts(t)
	testHTTPUploadMethod(t, "PUT", true, true, "", "")
}

func TestHTTPUploadPOSTWithoutChecksum(t *testing.T) {
	testHTTPUploadMethod(t, "POST", false, false, "", "")
}

func TestHTTPSUploadPOSTWithoutChecksum(t *testing.T) {
	setSSLCerts(t)
	defer unsetSSLCerts(t)
	testHTTPUploadMethod(t, "POST", false, true, "", "")
}

func TestHTTPUploadPOSTWithChecksum(t *testing.T) {
	testHTTPUploadMethod(t, "POST", true, false, "", "")
}

func TestHTTPSUploadPOSTWithCertAndKey(t *testing.T) {
	testHTTPUploadMethod(t, "POST", false, true, validCert, validKey)
}

func TestHTTPSUploadPOSTWithChecksum(t *testing.T) {
	setSSLCerts(t)
	defer unsetSSLCerts(t)
	testHTTPUploadMethod(t, "POST", true, true, "", "")
}

func TestNewHttpUploaderErrors(t *testing.T) {
	options := map[string]string{}

	u, err := NewHTTPUploader(options, "")
	assertNil(t, u)
	assertError(t, err)

	options[URLProp] = "http://localhost/up"
	options[MethodProp] = "GET"

	u, err = NewHTTPUploader(options, "")
	assertNil(t, u)
	assertError(t, err)
}

func TestHTTPUploadPortFailure(t *testing.T) {
	testHTTPUploadFailure(t, "http://localhost:5678/up", false)
}

func TestHTTPUploadAliasFailure(t *testing.T) {
	testHTTPUploadFailure(t, "http://localhost:1234/wrong", false)
}

func TestHTTPSUploadCertificateFailure(t *testing.T) {
	testHTTPUploadFailure(t, "https://localhost:2345/up", true)
}

func testHTTPUploadFailure(t *testing.T, addr string, secure bool) {
	f, err := os.Open(testFile)
	assertNoError(t, err)

	defer f.Close()
	defer handler.reset()

	options := map[string]string{
		URLProp: addr,
	}

	serverCert := ""
	if secure {
		serverCert = expiredCert
	}

	u, err := NewHTTPUploader(options, serverCert)
	assertNoError(t, err)

	err = u.UploadFile(f, false, nil)
	assertError(t, err)
}

func testHTTPUploadMethod(t *testing.T, method string, useChecksum bool, secure bool, cert string, key string) {
	f, err := os.Open(testFile)
	assertNoError(t, err)

	defer f.Close()
	defer handler.reset()

	options := map[string]string{}
	if secure {
		options[URLProp] = "https://localhost:2345/up"
	} else {
		options[URLProp] = "http://localhost:1234/up"
	}

	if method != "" {
		options[MethodProp] = method
	} else {
		method = "PUT"
	}

	addAll(options, prefixKeys(testHeaders, HeadersPrefix))

	u, err := NewHTTPUploader(options, cert)
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
