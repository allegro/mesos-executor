package vaas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	log "github.com/sirupsen/logrus"
)

// TaskStatus is a type representing task status from VaaS API responses.
type TaskStatus string

const (
	apiPrefixPath   = "/api/v0.1"
	apiBackendPath  = apiPrefixPath + "/backend/"
	apiDcPath       = apiPrefixPath + "/dc/"
	apiDirectorPath = apiPrefixPath + "/director/"

	// StatusPending Task state is unknown (assumed pending since you know the id).
	StatusPending = TaskStatus("PENDING")
	// StatusReceived Task was received by a worker (only used in events).
	StatusReceived = TaskStatus("RECEIVED")
	// StatusStarted Task was started by a worker (task_track_started).
	StatusStarted = TaskStatus("STARTED")
	// StatusSuccess Task succeeded
	StatusSuccess = TaskStatus("SUCCESS")
	// StatusFailure Task failed
	StatusFailure = TaskStatus("FAILURE")
	// StatusRevoked Task was revoked.
	StatusRevoked = TaskStatus("REVOKED")
	// StatusRetry Task is waiting for retry.
	StatusRetry = TaskStatus("RETRY")
)

const (
	contentTypeHeader = "Content-Type"
	acceptHeader      = "Accept"
	applicationJSON   = "application/json"
)

// Backend represents JSON structure of backend in VaaS API.
type Backend struct {
	ID                 *int     `json:"id,omitempty"`
	Address            string   `json:"address,omitempty"`
	Director           string   `json:"director,omitempty"`
	DC                 DC       `json:"dc,omitempty"`
	Port               int      `json:"port,omitempty"`
	InheritTimeProfile bool     `json:"inherit_time_profile,omitempty"`
	Weight             *int     `json:"weight,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	ResourceURI        string   `json:"resource_uri,omitempty"`
}

// DC represents JSON structure of DC in VaaS API.
type DC struct {
	ID          int    `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	ResourceURI string `json:"resource_uri,omitempty"`
	Symbol      string `json:"symbol,omitempty"`
}

// DCList represents JSON structure of DC list used in responses in VaaS API.
type DCList struct {
	Meta    Meta `json:"meta,omitempty"`
	Objects []DC `json:"objects,omitempty"`
}

// Director represents JSON structure of Director in VaaS API.
type Director struct {
	ID          int      `json:"id,omitempty"`
	Backends    []string `json:"backends,omitempty"`
	Cluster     []string `json:"cluster,omitempty"`
	Name        string   `json:"name,omitempty"`
	ResourceURI string   `json:"resource_uri,omitempty"`
}

// DirectorList represents JSON structure of Director list used in responses in VaaS API.
type DirectorList struct {
	Meta    Meta       `json:"meta,omitempty"`
	Objects []Director `json:"objects,omitempty"`
}

// Meta represents JSON structure of Meta in VaaS API.
type Meta struct {
	Limit      int     `json:"limit,omitempty"`
	Next       *string `json:"next,omitempty"`
	Offset     int     `json:"offset,omitempty"`
	Previous   *string `json:"previous,omitempty"`
	TotalCount int     `json:"total_count,omitempty"`
}

// Task represents JSON structure of a VaaS task in API.
type Task struct {
	Info        string     `json:"info,omitempty"`
	ResourceURI string     `json:"resource_uri,omitempty"`
	Status      TaskStatus `json:"status,omitempty"`
}

// Client is an interface for VaaS API.
type Client interface {
	FindDirectorID(string) (int, error)
	AddBackend(*Backend, bool) (string, error)
	DeleteBackend(int) error
	GetDC(string) (*DC, error)
	TaskStatus(*Task) error
}

// DefaultClient is a REST client for VaaS API.
type defaultClient struct {
	httpClient *http.Client
	username   string
	apiKey     string
	host       string
}

// FindDirectorID finds Director ID by name.
func (c *defaultClient) FindDirectorID(name string) (int, error) {
	request, err := c.newRequest("GET", c.host+apiDirectorPath, nil)
	if err != nil {
		return 0, err
	}

	query := request.URL.Query()
	query.Add("name", name)
	request.URL.RawQuery = query.Encode()

	var directorList DirectorList
	if _, err = c.doRequest(request, &directorList); err != nil {
		return 0, err
	}

	for _, director := range directorList.Objects {
		if director.Name == name {
			return director.ID, nil
		}
	}

	return 0, fmt.Errorf("no Director with name %s found", name)
}

// AddBackend adds backend in VaaS director.
func (c *defaultClient) AddBackend(backend *Backend, async bool) (string, error) {
	request, err := c.newRequest("POST", c.host+apiBackendPath, backend)
	if err != nil {
		return "", err
	}

	if async {
		request.Header.Set("Prefer", "respond-async")
	}

	response, err := c.doRequest(request, backend)
	if err != nil {
		return "", err
	}

	return response.Header.Get("Location"), nil
}

// DeleteBacked removes backend with given id from VaaS director.
func (c *defaultClient) DeleteBackend(id int) error {
	request, err := c.newRequest("DELETE", fmt.Sprintf("%s%s%d/", c.host, apiBackendPath, id), nil)
	if err != nil {
		return err
	}

	request.Header.Set("Prefer", "respond-async")
	response, err := c.do(request)
	if response != nil && response.StatusCode == http.StatusNotFound {
		log.WithField(vaasBackendIDKey, id).Warn("Tried to remove a non-existent backend")
		return nil
	}
	if err != nil {
		return err
	}

	request.Header.Set("Prefer", "respond-async")

	_, err = c.doRequest(request, nil)

	return err
}

// GetDC finds DC by name.
func (c *defaultClient) GetDC(name string) (*DC, error) {
	request, err := c.newRequest("GET", c.host+apiDcPath, nil)
	if err != nil {
		return nil, err
	}

	var dcList DCList
	if _, err := c.doRequest(request, &dcList); err != nil {
		return nil, err
	}

	for _, dc := range dcList.Objects {
		if dc.Symbol == name {
			return &dc, nil
		}
	}

	return nil, fmt.Errorf("no DC with name %s found", name)
}

func (c *defaultClient) TaskStatus(task *Task) error {
	request, err := c.newRequest("GET", c.host+task.ResourceURI, nil)
	if err != nil {
		return err
	}

	if _, err := c.doRequest(request, task); err != nil {
		return err
	}

	return nil
}

func (c *defaultClient) newRequest(method, url string, body interface{}) (*http.Request, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest(method, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	request.Header.Set(acceptHeader, applicationJSON)
	request.Header.Set(contentTypeHeader, applicationJSON)

	query := request.URL.Query()
	query.Add("username", c.username)
	query.Add("api_key", c.apiKey)
	request.URL.RawQuery = query.Encode()

	return request, nil
}

func (c *defaultClient) doRequest(request *http.Request, v interface{}) (*http.Response, error) {
	response, err := c.do(request)
	if err != nil {
		return response, err
	}

	rawResponse, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return response, err
	}

	if v == nil {
		return response, nil
	}
	if err := json.Unmarshal(rawResponse, v); err != nil {
		return response, err
	}

	return response, nil
}

func (c *defaultClient) do(request *http.Request) (*http.Response, error) {
	response, err := c.httpClient.Do(request)

	if err != nil {
		return response, err
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		message := ""
		rawResponse, err := ioutil.ReadAll(response.Body)
		if err != nil {
			message = fmt.Sprintf("Additional error reading raw response: %s", err.Error())
		} else {
			message = string(rawResponse)
		}
		return response, fmt.Errorf("VaaS API error at %s (HTTP %d): %s",
			request.URL, response.StatusCode, message)
	}

	return response, nil
}

// NewClient creates new REST client for VaaS API.
func NewClient(hostname string, username string, apiKey string) Client {
	return &defaultClient{
		httpClient: http.DefaultClient,
		username:   username,
		apiKey:     apiKey,
		host:       hostname,
	}
}
