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

package client

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eclipse-kanto/file-upload/uploaders"
)

const (
	minFileSize = 20000 // strings
	maxFileSize = 60000 // strings
)

func TestStatusFinished(t *testing.T) {
	s := UploadStatus{}

	finished := []string{StateSuccess, StateFailed, StateCanceled}
	for _, state := range finished {
		s.State = state
		if !s.finished() {
			t.Fatalf("%+v should be finished", s)
		}
	}

	ongoing := []string{StatePending, StateUploading, StatePaused}
	for _, state := range ongoing {
		s.State = state
		if s.finished() {
			t.Fatalf("%+v should not be finished", s)
		}
	}
}

type TestStatusListener struct {
	t *testing.T

	cond *sync.Cond

	finished bool

	status UploadStatus

	lastUploadProgress                int
	invalidUploadProgressErrorMessage string
}

func NewTestStatusListener(t *testing.T) *TestStatusListener {
	return &TestStatusListener{t, sync.NewCond(&sync.Mutex{}), false, UploadStatus{}, 0, ""}
}

func (l *TestStatusListener) uploadStatusUpdated(s *UploadStatus) {
	l.t.Logf("%+v\n", s)

	if s.Progress < 0 || s.Progress > 100 {
		l.invalidUploadProgressErrorMessage = fmt.Sprintf("upload progress must be between 0 and 100, but is %d", s.Progress)
	}
	if s.Progress < l.lastUploadProgress {
		l.invalidUploadProgressErrorMessage = fmt.Sprintf("upload progress value decreased(%d -> %d)", l.lastUploadProgress, s.Progress)
	}
	l.lastUploadProgress = s.Progress

	if s.finished() {
		l.cond.L.Lock()
		l.finished = true
		l.status = *s

		l.cond.Signal()
		l.cond.L.Unlock()
	}
}

func (l *TestStatusListener) getStatus() UploadStatus {
	l.cond.L.Lock()
	defer l.cond.L.Unlock()

	return l.status
}

func (l *TestStatusListener) waitFinish() {
	l.cond.L.Lock()
	for !l.finished {
		l.cond.Wait()
	}
	l.cond.L.Unlock()
}

func (l *TestStatusListener) isFinished() bool {
	l.cond.L.Lock()
	defer l.cond.L.Unlock()

	return l.finished
}

func (l *TestStatusListener) assertStatusState(expected string) {
	l.t.Helper()

	status := l.getStatus()
	if status.State != expected {
		l.t.Errorf("wrong status - expected %s, but was %s", expected, status.State)
	}
}

func TestRemoveChild(t *testing.T) {
	us := NewUploads()

	paths := []string{"t1.txt", "t2.txt"}
	ids := us.AddMulti("testUID", paths, false, false, "", NewTestStatusListener(t))

	for i, id := range ids {
		u := us.Get(id)

		if u == nil {
			t.Fatalf("upload with ID '%s' for path '%s' not found", id, paths[i])
		}

		us.Remove(id)

		if us.Get(id) != nil {
			t.Fatalf("upload '%s' still available after remove", id)
		}
	}
}

func TestRemoveParent(t *testing.T) {
	us := NewUploads()
	paths := []string{"t1.txt", "t2.txt"}

	const parentID = "testUID"
	ids := us.AddMulti(parentID, paths, false, false, "", NewTestStatusListener(t))

	us.Remove(parentID)

	if us.Get(parentID) != nil {
		t.Fatalf("upload '%s' still available after remove", parentID)
	}

	for _, id := range ids {
		if us.Get(id) != nil {
			t.Fatalf("child upload '%s' still available after parent '%s' is removed", id, parentID)
		}
	}
}

func TestSuccessHTTP(t *testing.T) {
	testSuccessfulUpload(t, false)
}

func TestSuccessHTTPS(t *testing.T) {
	testSuccessfulUpload(t, true)
}

func testSuccessfulUpload(t *testing.T, secure bool) {
	files := createTestFiles(t, 3, false, false)
	defer cleanFiles(files)

	server := startTestServer(t, 0, secure)
	defer server.Close()

	serverCert := ""
	if secure {
		cert, files, err := getTestServerSecureConfig(server)
		assertNoError(t, err)
		defer cleanFiles(files)

		serverCert = cert
	}

	us := NewUploads()
	paths := getPaths(files)

	l := NewTestStatusListener(t)
	ids := us.AddMulti("testUID", paths, true, false, serverCert, l)

	startUploads(t, us, ids, server.URL)

	l.waitFinish()
	l.assertStatusState(StateSuccess)

	if us.hasPendingUploads() {
		t.Error("no pending upload expected at this point")
	}

	status := l.getStatus()
	if status.Progress != 100 {
		t.Errorf("progress expected to be 100%%, but was %d%%", status.Progress)
	}
}

func TestUploadStatusOrder(t *testing.T) {
	cond := sync.NewCond(&sync.Mutex{})
	var finished bool
	var wrongStatusMsg string
	var lastUploadProgress int
	u := AutoUploadable{}

	u.statusEvents = newStatusEventsConsumer(100)
	u.statusEvents.start(func(e interface{}) {
		status := e.(UploadStatus)
		if status.Progress < lastUploadProgress {
			wrongStatusMsg = fmt.Sprintf("Upload progress value decreased(%d -> %d)", status.Progress, lastUploadProgress)
		}
		lastUploadProgress = status.Progress

		cond.L.Lock()
		defer cond.L.Unlock()

		if status.finished() {
			if status.State != StateSuccess {
				wrongStatusMsg = fmt.Sprintf("Upload failed with status %v", status)
			}
			finished = true
			cond.Signal()
		} else if finished {
			wrongStatusMsg = fmt.Sprintf("Received upload status %v after success", status)
		}

	})
	defer u.statusEvents.stop()

	files := createTestFiles(t, 60, true, false)
	defer cleanFiles(files)

	server := startTestServer(t, 0, false)
	defer server.Close()

	us := NewUploads()
	paths := getPaths(files)
	ids := us.AddMulti("testUID", paths, true, false, "", &u)
	startUploads(t, us, ids, server.URL)

	cond.L.Lock()
	for !finished {
		cond.Wait()
	}
	cond.L.Unlock()

	if wrongStatusMsg != "" {
		t.Error(wrongStatusMsg)
	}
}

func TestFailure(t *testing.T) {
	files := createTestFiles(t, 5, false, false)
	defer cleanFiles(files)

	us := NewUploads()
	paths := getPaths(files)
	paths[2] = "non-existing.grbg" //replace with non-existing file

	server := startTestServer(t, 0, false)
	defer server.Close()

	l := NewTestStatusListener(t)
	ids := us.AddMulti("testUID", paths, true, false, "", l)

	startUploads(t, us, ids, server.URL)

	time.Sleep(1 * time.Second)
	l.waitFinish()

	l.assertStatusState(StateFailed)
}

func TestCancel(t *testing.T) {
	const filesCount = 5
	files := createTestFiles(t, filesCount, false, false)
	defer cleanFiles(files)

	server := startTestServer(t, 50*time.Millisecond, false)
	defer server.Close()

	us := NewUploads()
	l := NewTestStatusListener(t)
	ids := us.AddMulti("testUID", getPaths(files), true, false, "", l)

	startUploads(t, us, ids, server.URL)

	const code = "tc"
	const msg = "test message"

	time.Sleep(10 * time.Millisecond)
	us.Get(ids[filesCount-1]).cancel(code, msg)

	l.waitFinish()
	l.assertStatusState(StateCanceled)

	status := l.getStatus()
	if status.StatusCode != code {
		t.Errorf("expected status code '%s', but was '%s", code, status.StatusCode)
	}

	if status.Message != msg {
		t.Errorf("expected status message '%s', but was '%s", msg, status.Message)
	}

	time.Sleep(2 * time.Second) //wait for uploads in progress
}

func TestGracefulShutdown(t *testing.T) {
	files := createTestFiles(t, 1, false, false)
	defer cleanFiles(files)

	delay := 1 * time.Second
	server := startTestServer(t, delay, false)

	defer server.Close()

	us := NewUploads()
	paths := getPaths(files)

	const parentID = "testUID"
	l := NewTestStatusListener(t)
	ids := us.AddMulti(parentID, paths, false, false, "", l)

	startUploads(t, us, ids, server.URL)

	if !us.hasPendingUploads() {
		t.Fatal("pending upload expected, but none found")
	}

	us.Stop(delay * 2)

	if !l.isFinished() {
		t.Fatal("all uploads should have finished")
	}
}

func TestProvidersErrors(t *testing.T) {
	us := NewUploads()
	ids := us.AddMulti("testUID", []string{"test.txt"}, false, false, "", nil)

	u := us.Get(ids[0])

	options := map[string]string{}

	options[StorageProvider] = "non-existing"
	if err := u.start(options); err == nil {
		t.Error("error for non-existing provider expected")
	}

	options[StorageProvider] = uploaders.StorageProviderAWS
	if err := u.start(options); err == nil {
		t.Error("error for missing AWS credentials expected")
	}

	options[StorageProvider] = uploaders.StorageProviderHTTP
	if err := u.start(options); err == nil {
		t.Error("error for missing upload URL expected")
	}
}

func getTestServerSecureConfig(server *httptest.Server) (string, []*os.File, error) {
	if server.TLS == nil {
		return "", nil, fmt.Errorf("no TLS configuration for test HTTPS server")
	}
	if len(server.TLS.Certificates) == 0 {
		return "", nil, fmt.Errorf("no TLS certificates for test HTTPS server")
	}

	serverCert := "testCert"

	files := make([]*os.File, 2)
	certFile, err := os.Create(serverCert)
	if err != nil {
		return "", nil, err
	}
	files[0] = certFile

	keyFile, err := os.Create("testKey")
	if err != nil {
		return "", files, err
	}
	files[1] = keyFile

	cert := server.TLS.Certificates[0]
	pk, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		return "", files, err
	}

	pemdataCert := pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Certificate[0],
		},
	)
	pemdataKey := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: pk,
		},
	)
	certFile.Write(pemdataCert)
	keyFile.Write(pemdataKey)

	return serverCert, files, nil
}

func startTestServer(t *testing.T, delay time.Duration, secure bool) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		time.Sleep(delay)

		if _, err := ioutil.ReadAll(r.Body); err != nil {
			t.Log(err)
		}
	})
	if secure {
		return httptest.NewTLSServer(handler)
	}
	return httptest.NewServer(handler)
}

func startUploads(t *testing.T, us *Uploads, ids []string, url string) {
	for _, id := range ids {

		if u := us.Get(id); u != nil {
			options := map[string]string{uploaders.URLProp: url}
			err := u.start(options)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			t.Logf("upload with ID '%s' not found", id)
		}
	}
}

func createTestFiles(t *testing.T, count int, randomSizedMediumFiles bool,
	emptyFiles bool) []*os.File {
	tmp := make([]*os.File, count)

	defer func() { //clean-up on error
		cleanFiles(tmp)
	}()

	for i := 0; i < count; i++ {
		if f, err := os.CreateTemp("./", "test"); err == nil {
			data := fmt.Sprintf("test %d", i+1)
			if emptyFiles {
				data = ""
			} else if randomSizedMediumFiles {
				rand.Seed(time.Now().UnixNano())
				data = strings.Repeat(data, rand.Intn(maxFileSize-minFileSize+1)+minFileSize)
			}

			if _, err := f.WriteString(data); err != nil {
				t.Fatal(err)
			}
			tmp[i] = f
		} else {
			t.Fatal(err)
		}
	}

	files := tmp
	tmp = nil

	return files
}

func getPaths(files []*os.File) []string {
	paths := make([]string, len(files))

	for i, f := range files {
		paths[i] = f.Name()
	}

	return paths
}

func cleanFiles(files []*os.File) {
	for _, f := range files {
		if f != nil {
			report(f.Close())
			report(os.Remove(f.Name()))
		}
	}
}

func report(err error) {
	if err != nil {
		log.Println(err)
	}
}
