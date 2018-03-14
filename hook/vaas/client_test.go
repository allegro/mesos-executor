package vaas

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoFailureWhenFindingDirectorByName(t *testing.T) {
	expectedID := 123
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, applicationJSON, r.Header.Get(contentTypeHeader))
		assert.Equal(t, applicationJSON, r.Header.Get(acceptHeader))
		if r.URL.Path == "/api/v0.1/director/" && r.URL.RawQuery == "api_key=api-key&name=director&username=username" && r.Method == "GET" {
			var dList = DirectorList{
				Objects: []Director{{ID: expectedID, Name: "director"}},
			}

			var data, _ = json.Marshal(dList)

			var _, err = w.Write(data)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
			}

			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "username", "api-key")

	directorID, err := client.FindDirectorID("director")

	require.NoError(t, err)
	assert.Equal(t, expectedID, directorID)
}

func TestFailureWhenFindingDirectorByName(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, applicationJSON, r.Header.Get(contentTypeHeader))
		assert.Equal(t, applicationJSON, r.Header.Get(acceptHeader))
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "username", "api-key")

	_, err := client.FindDirectorID("director")

	require.Error(t, err)
}

func TestBackendRegistrationFailureAfterVaasServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, applicationJSON, r.Header.Get(contentTypeHeader))
		assert.Equal(t, applicationJSON, r.Header.Get(acceptHeader))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "username", "api-key")

	_, err := client.AddBackend(&Backend{})

	assert.Error(t, err)
}

func TestBackendRemovalFailureAfterVaasServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, applicationJSON, r.Header.Get(contentTypeHeader))
		assert.Equal(t, applicationJSON, r.Header.Get(acceptHeader))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "username", "api-key")

	err := client.DeleteBackend(123)

	assert.Error(t, err)
}

func TestFindingDCWithNameFailureAfterVaasServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, applicationJSON, r.Header.Get(contentTypeHeader))
		assert.Equal(t, applicationJSON, r.Header.Get(acceptHeader))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "username", "api-key")

	_, err := client.GetDC("dc6")

	assert.Error(t, err)
}

func TestIfBackendLocationIsSetFromVaasResponseHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, applicationJSON, r.Header.Get(contentTypeHeader))
		assert.Equal(t, applicationJSON, r.Header.Get(acceptHeader))
		if r.URL.Path == "/api/v0.1/backend/" && r.Method == "POST" {
			rawRequest, err := ioutil.ReadAll(r.Body)

			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			var b Backend

			if err := json.Unmarshal(rawRequest, &b); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.Header().Set("Location", "location")
		} else {
			w.WriteHeader(http.StatusNotFound)
		}

		w.Write(mockAddBackendResponse)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "username", "api-key")

	location, err := client.AddBackend(&Backend{
		Address:  "127.0.0.1",
		Director: "director",
		DC:       DC{1, "DC1", "api/dc/1", "dc1"},
		Port:     8080,
	})

	require.NoError(t, err)
	assert.Equal(t, "location", location)
}

func TestNoFailureWhenRemovingExistingBackendInVaas(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, applicationJSON, r.Header.Get(contentTypeHeader))
		assert.Equal(t, applicationJSON, r.Header.Get(acceptHeader))
		if r.URL.Path == "/api/v0.1/backend/id/" && r.Method == "DELETE" {
			w.WriteHeader(http.StatusAccepted)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "username", "api-key")

	err := client.DeleteBackend(123)

	assert.NoError(t, err)
}

func TestNoFailureWhenRemovingNonExistingBackendInVaas(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, applicationJSON, r.Header.Get(contentTypeHeader))
		assert.Equal(t, applicationJSON, r.Header.Get(acceptHeader))
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "username", "api-key")

	err := client.DeleteBackend(123)

	assert.NoError(t, err)
}

var mockAddBackendResponse = []byte(`{
   "address":"192.168.199.34",
   "between_bytes_timeout":"1",
   "connect_timeout":"0.3",
   "dc":{
	  "id":1,
	  "name":"First datacenter",
	  "resource_uri":"/api/v0.1/dc/1/",
	  "symbol":"dc1"
   },
   "director":"/api/v0.1/director/1/",
   "enabled":true,
   "first_byte_timeout":"5",
   "id":1,
   "inherit_time_profile":true,
   "max_connections":5,
   "port":80,
   "resource_uri":"/api/v0.1/backend/1/",
   "status":"Unknown",
   "tags":[],
   "time_profile":{
	  "between_bytes_timeout":"1",
	  "connect_timeout":"0.3",
	  "first_byte_timeout":"5",
	  "max_connections":5
   },
   "weight":1
}`)
