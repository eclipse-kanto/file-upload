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
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eclipse-kanto/file-upload/logger"
	"github.com/eclipse-kanto/file-upload/uploaders"
)

// Constants for AutoUploadable 'state' property values
const (
	StatePending   = "PENDING"
	StateUploading = "UPLOADING"
	StatePaused    = "PAUSED"
	StateSuccess   = "SUCCESS"
	StateFailed    = "FAILED"
	StateCanceled  = "CANCELED"
)

// InfoPrefix is used to prefix properties in start options, which should be included
// (with the prefix removed) in the upload status 'info' property.
const InfoPrefix = "info."

// StorageProvider hold the name of the storage provider 'start' operation option
const StorageProvider = "storage.provider"

// fineGrainedUploadProgressNotSupported indicates, that at least file size cannot be determined and upload progress will be based on file count only
const fineGrainedUploadProgressNotSupported = -1

// Upload represents single or multi-file upload
type Upload interface {
	start(options map[string]string) error
	cancel(code string, message string)
}

// MultiUpload represents a multi-file upload.
type MultiUpload struct {
	correlationID string

	children   map[string]*SingleUpload
	totalCount int

	deleteUploaded bool
	useChecksum    bool

	serverCert string

	uploads *Uploads

	status   *UploadStatus
	listener UploadStatusListener

	mutex sync.RWMutex

	totalBytesTransferred int64
	totalSizeBytes        int64 // -1(fineGrainedUploadProgressNotSupported) if there is an error, retrieving at least one file size(file count progress report will be used in such case)
}

// SingleUpload represents a single file upload
type SingleUpload struct {
	correlationID string
	filePath      string
	parent        *MultiUpload

	started uint32
	file    *os.File
	mutex   sync.RWMutex

	bytesTransferred int64 //always 0 if uploader does not call back listener for number of uploaded bytes
	totalSizeBytes   int64
}

// Uploads maps correlation IDs to Upload instances
type Uploads struct {
	mutex sync.RWMutex

	uploads map[string]Upload
}

// UploadStatus is used for serializing the 'status' property of the AutoUploadable feature
type UploadStatus struct {
	CorrelationID string `json:"correlationId"`
	State         string `json:"state"`

	StartTime  time.Time `json:"startTime"`
	EndTime    time.Time `json:"endTime"`
	StatusCode string    `json:"statusCode"`
	Message    string    `json:"message"`

	Progress int `json:"progress"`

	Info map[string]string `json:"info"`
}

func (s *UploadStatus) finished() bool {
	return s.State == StateSuccess || s.State == StateCanceled || s.State == StateFailed
}

// UploadStatusListener is notified on changes in uploads status
type UploadStatusListener interface {
	uploadStatusUpdated(status *UploadStatus) // should not block
}

//******* Uploads methods *******//

// NewUploads constructs new Uploads instance
func NewUploads() *Uploads {
	r := &Uploads{}

	r.uploads = make(map[string]Upload)

	return r
}

// AddMulti is used to add an upload, containing multiple files. The provided listener will be notified on the upload progress.
// If deleteUploaded is true, files will be deleted after successful upload.
func (us *Uploads) AddMulti(correlationID string, paths []string, deleteUploaded bool, useChecksum bool,
	serverCert string, listener UploadStatusListener) []string {

	m := &MultiUpload{}
	m.correlationID = correlationID
	m.listener = listener
	m.deleteUploaded = deleteUploaded
	m.useChecksum = useChecksum
	m.serverCert = serverCert
	m.totalCount = len(paths)
	m.children = make(map[string]*SingleUpload)
	m.uploads = us

	r := make([]string, len(paths))
	for i, path := range paths {
		id := fmt.Sprintf("%s#%d", correlationID, i+1)

		us.AddSingle(m, id, path)

		r[i] = id

		if m.totalSizeBytes != fineGrainedUploadProgressNotSupported {
			fileInfo, err := os.Stat(path)
			if err != nil {
				logger.Warnf("cannot get size of file %s", path)
				m.totalSizeBytes = fineGrainedUploadProgressNotSupported // will use progress report, based on number of uploaded files
			} else {
				size := fileInfo.Size()
				us.mutex.Lock()
				m.totalSizeBytes += size
				m.children[id].totalSizeBytes = size
				us.mutex.Unlock()
			}
		}
	}

	us.mutex.Lock()
	defer us.mutex.Unlock()
	us.uploads[correlationID] = m

	return r
}

// AddSingle adds single file upload to a MultiUpload
func (us *Uploads) AddSingle(parent *MultiUpload, correlationID string, filePath string) {
	u := &SingleUpload{}
	u.correlationID = correlationID
	u.filePath = filePath
	u.parent = parent

	parent.addChild(u)

	us.mutex.Lock()
	defer us.mutex.Unlock()
	us.uploads[correlationID] = u
}

// Get returns the upload with the given correlation ID or nil, if the correlation ID is unknown
func (us *Uploads) Get(correlationID string) Upload {
	us.mutex.RLock()
	defer us.mutex.RUnlock()

	u, ok := us.uploads[correlationID]

	if ok {
		return u
	}

	return nil
}

// Remove deles the upload with the given correlation ID and its children (if any).
// If the correlation ID is not known - nothing is done.
func (us *Uploads) Remove(correlationID string) {
	us.mutex.Lock()
	defer us.mutex.Unlock()

	u, ok := us.uploads[correlationID]
	if !ok {
		return
	}

	delete(us.uploads, correlationID)

	mu, ok := u.(*MultiUpload)
	if ok {
		childrenIDs := mu.getChildrenIDs()
		for _, childID := range childrenIDs {
			delete(us.uploads, childID)
		}
	}
}

// Stop waits for pending uploads to complete in the given timeout. Uploads which are still
// pending after the timeout are canceled.
func (us *Uploads) Stop(timeout time.Duration) {
	logger.Info("waiting for pending uploads...")
	end := time.Now().Add(timeout)

	pending := true
	for pending && time.Now().Before(end) {
		pending = us.hasPendingUploads()

		if pending {
			time.Sleep(2 * time.Second)
		}
	}

	logger.Info("cancelling pending uploads...")
	if pending {
		us.mutex.Lock()
		defer us.mutex.Unlock()
		for _, u := range us.uploads {
			mu, ok := u.(*MultiUpload)

			if ok {
				mu.cancelUploads()
			}
		}
	}
}

func (us *Uploads) hasPendingUploads() bool {
	us.mutex.RLock()
	defer us.mutex.RUnlock()

	for _, u := range us.uploads {
		mu, ok := u.(*MultiUpload)

		if ok && mu.status != nil && mu.status.State == StateUploading {
			return true
		}
	}

	return false
}

//******* END Uploads methods *******//

//******* MultiUpload methods *******//

func (u *MultiUpload) addChild(su *SingleUpload) {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	u.children[su.correlationID] = su
}

func (u *MultiUpload) removeChild(su *SingleUpload) {
	defer u.uploads.Remove(su.correlationID)

	u.mutex.Lock()
	defer u.mutex.Unlock()

	delete(u.children, su.correlationID)
}

func (u *MultiUpload) getChildrenIDs() []string {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	ids := make([]string, 0, len(u.children))
	for id := range u.children {
		ids = append(ids, id)
	}

	return ids
}

func (u *MultiUpload) changeProgress(newBytesTransferred int64) {
	u.mutex.Lock()
	defer u.mutex.Unlock()
	if u.totalSizeBytes == 0 { //an empty file set, nothing to change
		if newBytesTransferred != 0 {
			logger.Warnf("reporting non-zero transferred bytes(%d) on an empty file set", newBytesTransferred)
		}
	} else if u.totalSizeBytes != fineGrainedUploadProgressNotSupported {
		u.totalBytesTransferred += newBytesTransferred
		newProgress := int((100 * float64(u.totalBytesTransferred)) / float64(u.totalSizeBytes))
		notify := newProgress != u.status.Progress
		u.status.Progress = newProgress
		if notify {
			u.listener.uploadStatusUpdated(u.status)
		}
	}

}

func (u *MultiUpload) start(options map[string]string) error {
	return fmt.Errorf("multi-file upload '%s' cannot be started - start the individual uploads", u.correlationID)
}

func (u *MultiUpload) cancel(code string, message string) {
	logger.Infof("multi-upload %s cancelled - code: %s, message: %s", u.correlationID, code, message)

	done := func() bool {
		u.mutex.Lock()
		defer u.mutex.Unlock()

		if u.status == nil { //not yet started
			u.status = &UploadStatus{}
		} else if u.status.finished() {
			return true
		}

		u.status.State = StateCanceled
		u.status.StatusCode = code
		u.status.Message = message
		u.status.EndTime = time.Now()
		u.listener.uploadStatusUpdated(u.status)

		return false
	}()

	if !done {
		u.cancelUploads()

		u.uploads.Remove(u.correlationID)

	}
}

func (u *MultiUpload) uploadStarted(su *SingleUpload, info map[string]string) {
	logger.Infof("upload %v started", su)

	u.mutex.Lock()
	defer u.mutex.Unlock()

	if u.status != nil && u.status.State != StatePending {
		return // already started
	}
	u.status = &UploadStatus{}
	u.status.CorrelationID = u.correlationID
	u.status.State = StateUploading
	u.status.StartTime = time.Now()
	u.status.Progress = 0
	u.status.Info = info
	u.status = &UploadStatus{}
	u.status.CorrelationID = u.correlationID
	u.status.State = StateUploading
	u.status.StartTime = time.Now()
	u.status.Progress = 0
	u.status.Info = info

	u.listener.uploadStatusUpdated(u.status)
}

func (u *MultiUpload) uploadFailed(su *SingleUpload, err error) {
	logger.Errorf("upload %v failed: %v", su, err)

	u.removeChild(su)

	done := func() bool {
		u.mutex.Lock()
		defer u.mutex.Unlock()

		if u.status == nil || u.status.finished() {
			return true
		}

		u.status.State = StateFailed
		u.status.EndTime = time.Now()
		u.status.Message = err.Error()
		u.listener.uploadStatusUpdated(u.status)

		return false
	}()

	if !done {
		u.cancelUploads()

		u.uploads.Remove(u.correlationID)

	}
}

func (u *MultiUpload) uploadFinished(su *SingleUpload) {
	logger.Infof("upload %v finished'", su)

	u.removeChild(su)

	done := func() bool {
		u.mutex.Lock()
		defer u.mutex.Unlock()

		if u.status.finished() {
			return false
		}

		remaining := len(u.children)
		if remaining == 0 {
			u.status.Progress = 100
			u.status.State = StateSuccess
			u.status.EndTime = time.Now()
		} else if u.totalSizeBytes != fineGrainedUploadProgressNotSupported && u.totalSizeBytes != 0 {
			u.totalBytesTransferred += su.totalSizeBytes - su.bytesTransferred // ensures that the total number of transferred bytes for a single file will be exactly its size
			u.status.Progress = int(100 * (float64(u.totalBytesTransferred) / float64(u.totalSizeBytes)))
		} else {
			uploaded := float32(u.totalCount - remaining)
			percents := 100 * (uploaded / float32(u.totalCount))
			u.status.Progress = int(percents)
		}

		u.listener.uploadStatusUpdated(u.status)

		return remaining == 0
	}()

	if done {
		u.uploads.Remove(u.correlationID)
	}

}

func (u *MultiUpload) uploadCancelled(su *SingleUpload, code string, message string) {
	logger.Infof("upload %v cancelled", su)

	u.removeChild(su)

	u.cancel(code, message) //cancel all uploads
}

func (u *MultiUpload) cancelUploads() {
	u.mutex.Lock()
	uploads := make([]*SingleUpload, 0, len(u.children))
	for _, su := range u.children {
		uploads = append(uploads, su)
	}
	u.mutex.Unlock()

	for _, su := range uploads {
		su.internalCancel()
		logger.Infof("upload %v cancelled", su)
	}
}

//******* END MultiUpload methods *******//

//******* SingleUpload methods *******//

func (u *SingleUpload) String() string {
	return fmt.Sprintf("[correlationID: %s, file: %s]", u.correlationID, u.filePath)
}

func (u *SingleUpload) start(options map[string]string) error {
	uploader, err := getUploader(options, u.parent.serverCert)

	if err != nil {
		return err
	}

	ok := atomic.CompareAndSwapUint32(&u.started, 0, 1)

	if !ok {
		return fmt.Errorf("upload '%s' already started", u.correlationID)
	}

	info := uploaders.ExtractDictionary(options, InfoPrefix)
	u.parent.uploadStarted(u, info)

	progressFunc := func(bytesTransferred int64) {
		if u.parent.totalSizeBytes == fineGrainedUploadProgressNotSupported {
			return // unsupported
		}
		if u.totalSizeBytes == 0 && bytesTransferred != 0 {
			logger.Warnf("reporting non-zero transferred bytes(%d) on an empty file(%v)", bytesTransferred, u.file)
			return
		}
		if u.bytesTransferred != bytesTransferred {
			change := bytesTransferred - u.bytesTransferred
			u.bytesTransferred = bytesTransferred
			if change != 0 {
				u.parent.changeProgress(change)
			}
		}
	}

	go func() {
		file, err := os.Open(u.filePath)
		var useChecksum bool

		if err == nil && u.parent.useChecksum {
			useChecksum = true
		}

		if err == nil {
			defer func() {
				file.Close()
			}()

			u.mutex.Lock()
			u.file = file
			u.mutex.Unlock()

			err = uploader.UploadFile(file, useChecksum, progressFunc)
		}

		if err != nil {
			u.parent.uploadFailed(u, err)
		} else {
			u.parent.uploadFinished(u)

			if u.parent.deleteUploaded {
				file.Close()
				err := os.Remove(u.filePath)

				if err != nil {
					logger.Errorf("failed to delete uploaded file '%s': %v", u.filePath, err)
				} else {
					logger.Infof("uploaded file '%s' deleted", u.filePath)
				}
			}
		}
	}()

	return nil
}

func getUploader(options map[string]string, serverCert string) (uploaders.Uploader, error) {
	storage, ok := options[StorageProvider]

	storage = strings.ToLower(storage)

	if !ok || storage == uploaders.StorageProviderHTTP {
		return uploaders.NewHTTPUploader(options, serverCert)
	} else if storage == uploaders.StorageProviderAWS {
		return uploaders.NewAWSUploader(options)
	} else if storage == uploaders.StorageProviderAzure {
		return uploaders.NewAzureUploader(options)
	}

	return nil, fmt.Errorf("unknown storage provider '%s'", storage)
}

func (u *SingleUpload) cancel(code string, message string) {
	u.internalCancel()

	u.parent.uploadCancelled(u, code, message)
}

func (u *SingleUpload) internalCancel() {
	var file *os.File

	u.mutex.RLock()
	file = u.file
	u.mutex.RUnlock()

	if file != nil {
		err := file.Close()

		if !errors.Is(err, os.ErrClosed) {
			logger.Errorf("failed to close file '%s'", u.filePath)
		}
	}
}

//******* END SingleUpload methods *******//
