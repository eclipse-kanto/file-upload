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
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/eclipse-kanto/file-upload/uploaders"
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
}

func NewTestStatusListener(t *testing.T) *TestStatusListener {
	return &TestStatusListener{t, sync.NewCond(&sync.Mutex{}), false, UploadStatus{}}
}

func (l *TestStatusListener) uploadStatusUpdated(s *UploadStatus) {
	l.t.Logf("%+v\n", s)

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
	ids := us.AddMulti("testUID", paths, false, false, NewTestStatusListener(t))

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
	ids := us.AddMulti(parentID, paths, false, false, NewTestStatusListener(t))

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

func TestSuccess(t *testing.T) {
	files := createTestFiles(t, 3)
	defer cleanFiles(files)

	server := startTestServer(t, 0)

	defer server.Close()

	us := NewUploads()
	paths := getPaths(files)

	l := NewTestStatusListener(t)
	ids := us.AddMulti("testUID", paths, true, false, l)

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

func TestFailure(t *testing.T) {
	files := createTestFiles(t, 5)
	defer cleanFiles(files)

	us := NewUploads()
	paths := getPaths(files)
	paths[2] = "non-existing.grbg" //replace with non-existing file

	server := startTestServer(t, 0)
	defer server.Close()

	l := NewTestStatusListener(t)
	ids := us.AddMulti("testUID", paths, true, false, l)

	startUploads(t, us, ids, server.URL)

	time.Sleep(1 * time.Second)
	l.waitFinish()

	l.assertStatusState(StateFailed)
}

func TestCancel(t *testing.T) {
	const filesCount = 5
	files := createTestFiles(t, filesCount)
	defer cleanFiles(files)

	server := startTestServer(t, 50*time.Millisecond)
	defer server.Close()

	us := NewUploads()
	l := NewTestStatusListener(t)
	ids := us.AddMulti("testUID", getPaths(files), true, false, l)

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
	files := createTestFiles(t, 1)
	defer cleanFiles(files)

	delay := 1 * time.Second
	server := startTestServer(t, delay)

	defer server.Close()

	us := NewUploads()
	paths := getPaths(files)

	const parentID = "testUID"
	l := NewTestStatusListener(t)
	ids := us.AddMulti(parentID, paths, false, false, l)

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
	ids := us.AddMulti("testUID", []string{"test.txt"}, false, false, nil)

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

func startTestServer(t *testing.T, delay time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		time.Sleep(delay)

		if _, err := ioutil.ReadAll(r.Body); err != nil {
			t.Log(err)
		}
	}))
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

func createTestFiles(t *testing.T, count int) []*os.File {
	tmp := make([]*os.File, count)

	defer func() { //clean-up on error
		cleanFiles(tmp)
	}()

	for i := 0; i < count; i++ {
		if f, err := os.CreateTemp("./", "test"); err == nil {
			if _, err := f.WriteString(fmt.Sprintf("test %d", i+1)); err != nil {
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
		report(f.Close())
		report(os.Remove(f.Name()))
	}
}

func report(err error) {
	if err != nil {
		log.Println(err)
	}
}
