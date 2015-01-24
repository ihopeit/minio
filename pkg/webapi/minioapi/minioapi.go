/*
 * Mini Object Storage, (C) 2014 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package minioapi

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	mstorage "github.com/minio-io/minio/pkg/storage"
)

type contentType int

const (
	xmlType  contentType = iota
	jsonType             = iota
)

const (
	dateFormat = "2006-01-02T15:04:05.000Z"
)

type minioApi struct {
	storage mstorage.Storage
}

// No encoder interface exists, so we create one.
type encoder interface {
	Encode(v interface{}) error
}

func HttpHandler(storage mstorage.Storage) http.Handler {
	mux := mux.NewRouter()
	api := minioApi{
		storage: storage,
	}

	mux.HandleFunc("/", api.listBucketsHandler).Methods("GET")
	mux.HandleFunc("/{bucket}", api.listObjectsHandler).Methods("GET")
	mux.HandleFunc("/{bucket}", api.putBucketHandler).Methods("PUT")
	mux.HandleFunc("/{bucket}/", api.listObjectsHandler).Methods("GET")
	mux.HandleFunc("/{bucket}/{object:.*}", api.getObjectHandler).Methods("GET")
	mux.HandleFunc("/{bucket}/{object:.*}", api.headObjectHandler).Methods("HEAD")
	mux.HandleFunc("/{bucket}/{object:.*}", api.putObjectHandler).Methods("PUT")
	return mux
}

func writeObjectHeaders(w http.ResponseWriter, metadata mstorage.ObjectMetadata) {
	lastModified := metadata.Created.Format(time.RFC1123)
	w.Header().Set("ETag", metadata.ETag)
	w.Header().Set("Last-Modified", lastModified)
	w.Header().Set("Content-Length", strconv.Itoa(metadata.Size))
	w.Header().Set("Content-Type", "text/plain")
}

func (server *minioApi) getObjectHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	bucket := vars["bucket"]
	object := vars["object"]

	metadata, err := server.storage.GetObjectMetadata(bucket, object)
	switch err := err.(type) {
	case nil: // success
		{
			log.Println("Found: " + bucket + "#" + object)
			writeObjectHeaders(w, metadata)
			if _, err := server.storage.CopyObjectToWriter(w, bucket, object); err != nil {
				log.Println(err)
			}
		}
	case mstorage.ObjectNotFound:
		{
			log.Println(err)
			w.WriteHeader(http.StatusNotFound)
		}
	default:
		{
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

func (server *minioApi) headObjectHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	bucket := vars["bucket"]
	object := vars["object"]

	metadata, err := server.storage.GetObjectMetadata(bucket, object)
	switch err := err.(type) {
	case nil:
		writeObjectHeaders(w, metadata)
	case mstorage.ObjectNotFound:
		log.Println(err)
		w.WriteHeader(http.StatusNotFound)
	default:
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
	}
}

func (server *minioApi) listBucketsHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	prefix, ok := vars["prefix"]
	if ok == false {
		prefix = ""
	}

	contentType := xmlType
	if _, ok := req.Header["Accept"]; ok {
		if req.Header["Accept"][0] == "application/json" {
			contentType = jsonType
		}
	}
	buckets := server.storage.ListBuckets(prefix)
	response := generateBucketsListResult(buckets)

	var bytesBuffer bytes.Buffer
	var encoder encoder
	if contentType == xmlType {
		w.Header().Set("Content-Type", "application/xml")
		encoder = xml.NewEncoder(&bytesBuffer)
	} else if contentType == jsonType {
		w.Header().Set("Content-Type", "application/json")
		encoder = json.NewEncoder(&bytesBuffer)
	}
	encoder.Encode(response)
	w.Write(bytesBuffer.Bytes())
}

func (server *minioApi) listObjectsHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)

	//delimiter, ok := vars["delimiter"]
	//encodingType, ok := vars["encoding-type"]
	//marker, ok := vars["marker"]
	//maxKeys, ok := vars["max-keys"]
	bucket := vars["bucket"]
	//bucket, ok := vars["bucket"]
	//if ok == false {
	//	w.WriteHeader(http.StatusBadRequest)
	//	return
	//}
	prefix, ok := vars["prefix"]
	if ok == false {
		prefix = ""
	}

	contentType := xmlType
	if _, ok := req.Header["Accept"]; ok {
		if req.Header["Accept"][0] == "application/json" {
			contentType = jsonType
		}
	}

	objects := server.storage.ListObjects(bucket, prefix, 1000)
	response := generateObjectsListResult(bucket, objects)

	var bytesBuffer bytes.Buffer
	var encoder encoder
	if contentType == xmlType {
		w.Header().Set("Content-Type", "application/xml")
		encoder = xml.NewEncoder(&bytesBuffer)
	} else if contentType == jsonType {
		w.Header().Set("Content-Type", "application/json")
		encoder = json.NewEncoder(&bytesBuffer)
	}

	encoder.Encode(response)
	w.Write(bytesBuffer.Bytes())
}

func (server *minioApi) putObjectHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	bucket := vars["bucket"]
	object := vars["object"]
	err := server.storage.StoreObject(bucket, object, req.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
}

func (server *minioApi) putBucketHandler(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	bucket := vars["bucket"]
	err := server.storage.StoreBucket(bucket)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
}

func generateBucketsListResult(buckets []mstorage.BucketMetadata) (data BucketListResponse) {
	var listbuckets []*Bucket

	owner := Owner{
		ID:          "minio",
		DisplayName: "minio",
	}

	for _, bucket := range buckets {
		listbucket := &Bucket{
			Name:         bucket.Name,
			CreationDate: bucket.Created.Format(dateFormat),
		}
		listbuckets = append(listbuckets, listbucket)
	}

	data = BucketListResponse{
		Owner: owner,
	}
	data.Buckets.Bucket = listbuckets
	return
}

func generateObjectsListResult(bucket string, objects []mstorage.ObjectMetadata) (data ObjectListResponse) {
	var contents []*Item

	owner := Owner{
		ID:          "minio",
		DisplayName: "minio",
	}

	for _, object := range objects {
		content := &Item{
			Key:          object.Key,
			LastModified: object.Created.Format(dateFormat),
			ETag:         object.ETag,
			Size:         object.Size,
			StorageClass: "STANDARD",
			Owner:        owner,
		}
		contents = append(contents, content)
	}
	data = ObjectListResponse{
		Name:        bucket,
		Contents:    contents,
		MaxKeys:     MAX_OBJECT_LIST,
		IsTruncated: false,
	}
	return
}