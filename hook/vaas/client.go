package vaas

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"gopkg.in/h2non/gentleman.v1"
	"gopkg.in/h2non/gentleman.v1/plugins/body"
	"gopkg.in/h2non/gentleman.v1/plugins/bodytype"
	"gopkg.in/h2non/gentleman.v1/plugins/query"

	log "github.com/Sirupsen/logrus"
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
	httpClient gentleman.Client
}

// FindDirectorID finds Director ID by name.
func (c *defaultClient) FindDirectorID(name string) (int, error) {
	request := c.httpClient.Request()
	request.Path(apiDirectorPath)
	request.Method("GET")
	request.AddQuery("name", name)

	response, err := c.doRequest(request)

	if err != nil {
		return 0, err
	}

	var directorList DirectorList

	if err := response.JSON(&directorList); err != nil {
		return 0, err
	}

	for _, director := range directorList.Objects {
		if director.Name == name {
			return director.ID, nil
		}
	}

	return 0, fmt.Errorf("No Director with name %s found", name)
}

// AddBackend adds backend in VaaS director.
func (c *defaultClient) AddBackend(backend *Backend, async bool) (string, error) {
	request := c.httpClient.Request()
	request.Path(apiBackendPath)
	request.Method("POST")
	request.Use(body.JSON(backend))
	if async {
		request.AddHeader("Prefer", "respond-async")
	}

	response, err := c.doRequest(request)

	if err != nil {
		return "", err
	}
	if err := response.JSON(&backend); err != nil {
		return "", err
	}

	return response.Header.Get("Location"), nil
}

// DeleteBacked removes backend with given id from VaaS director.
func (c *defaultClient) DeleteBackend(id int) error {
	request := c.httpClient.Request()
	request.Path(fmt.Sprintf("%s%d/", apiBackendPath, id))
	request.Method("DELETE")
	request.SetHeader("Prefer", "respond-async")

	response, err := c.doRequest(request)

	if response.StatusCode == http.StatusNotFound {
		log.WithField(vaasBackendIDKey, id).
			Warn("Tried to remove a non-existent backend")
		return nil
	}

	return err
}

// GetDC finds DC by name.
func (c *defaultClient) GetDC(name string) (*DC, error) {
	request := c.httpClient.Request()
	request.Path(apiDcPath)
	request.Method("GET")

	response, err := c.doRequest(request)

	if err != nil {
		return nil, err
	}

	var dcList DCList

	if err := response.JSON(&dcList); err != nil {
		return nil, err
	}

	for _, dc := range dcList.Objects {
		if dc.Symbol == name {
			return &dc, nil
		}
	}

	return nil, fmt.Errorf("No DC with name %s found", name)
}

func (c *defaultClient) TaskStatus(task *Task) error {
	request := c.httpClient.Request()
	request.Method("GET")
	request.Path(task.ResourceURI)

	response, err := c.doRequest(request)
	if err != nil {
		return err
	}

	if !response.Ok {
		message := ""
		rawResponse, err := ioutil.ReadAll(response.RawResponse.Body)
		if err != nil {
			message = fmt.Sprintf("Additional error reading raw response: %s", err.Error())
		} else {
			message = string(rawResponse)
		}
		return fmt.Errorf("VaaS API error at %s (HTTP %d): %s",
			request.Context.Request.URL, response.StatusCode, message)
	}

	return response.JSON(task)
}

func (c *defaultClient) doRequest(request *gentleman.Request) (*gentleman.Response, error) {
	response, err := request.Send()

	if err != nil {
		return nil, err
	}

	if !response.Ok {
		rawResponse, _ := ioutil.ReadAll(response.RawResponse.Body)
		return response, fmt.Errorf("VaaS API error at %s (HTTP %d): %s",
			request.Context.Request.URL, response.StatusCode, rawResponse)
	}

	return response, nil
}

// NewClient creates new REST client for VaaS API.
func NewClient(hostname string, username string, apiKey string) Client {
	httpClient := gentleman.New()
	httpClient.URL(hostname)
	httpClient.Use(query.SetMap(map[string]string{
		"username": username,
		"api_key":  apiKey,
	}))
	httpClient.Use(bodytype.Set("json"))

	return &defaultClient{
		httpClient: *httpClient,
	}
}
