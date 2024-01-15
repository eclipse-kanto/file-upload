[![Kanto logo](https://github.com/eclipse-kanto/kanto/raw/main/logo/kanto.svg)](https://eclipse.dev/kanto/)

# Eclipse Kanto - File Upload

[![Coverage](https://github.com/eclipse-kanto/file-upload/wiki/coverage.svg)](#)

## Overview

File upload between the edge and the cloud backend can enable a variety of use cases related to edge diagnostics and monitoring, as well as system backup and restore.

The file upload functionality gives the ability to configure the edge from the backend to send files periodically, or for the backend to explicitly trigger file upload from the device. 

Files can be uploaded to different storage providers, currently including AWS and standard HTTP upload.

File Upload implements the [AutoUploadable](https://github.com/eclipse/vorto/tree/development/models/com.bosch.iot.suite.manager.upload-AutoUploadable-1.0.0.fbmodel) Vorto model.

### Capabilities include:

 * HTTP upload - HTTP file upload, using backend provided pre-signed URL and authentications headers.
 * AWS upload - upload through AWS SDK, using backend provided AWS temporary credentials.
 * Periodic uploads - periodically trigger uploads at specified intervals.
 * Activity period - schedule periodic uploads for specified time frame.
 * Files filter - select files to be uploaded using glob pattern.
 * Delete uploaded - delete locally files which were successfully uploaded.

## Community

* [GitHub Issues](https://github.com/eclipse-kanto/file-upload/issues)
* [Mailing List](https://accounts.eclipse.org/mailing-list/kanto-dev)
