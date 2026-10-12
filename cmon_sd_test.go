package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/severalnines/cmon-proxy/cmon/api"
)

type CmonClientMock struct {
	Fail bool
}

func (c CmonClientMock) Authenticate() error {
	return nil
}

func (c CmonClientMock) ControllerID() string {
	return "00000000-0000-0000-0000-000000000000"
}

func (c CmonClientMock) GetAllClusterInfo(req *api.GetAllClusterInfoRequest) (*api.GetAllClusterInfoResponse, error) {
	if c.Fail {
		return nil, errors.New("cmon client error")
	}

	if !req.WithHosts || !req.WithTags {
		return nil, errors.New("expected with_hosts and with_tags")
	}

	return &api.GetAllClusterInfoResponse{
		Clusters: []*api.Cluster{
			{
				ClusterID:   1,
				ClusterName: "cluster1",
				ClusterType: "postgresql_single",
				Tags:        []string{"tenant=acme", "production"},
				Hosts: []*api.Host{
					{
						Nodetype: "mysql",
						IP:       "127.0.0.1",
					},
					{
						Nodetype: "mongo",
						IP:       "127.0.0.1",
						Role:     "mongos",
					},
				},
			},
		},
	}, nil
}

func TestHandlerIndexHandlerFail(t *testing.T) {
	cmonMock := CmonClientMock{
		Fail: true,
	}

	b := bytes.NewBuffer(nil)
	logger := slog.New(slog.NewJSONHandler(b, nil))

	s := Service{
		cmonClient: cmonMock,
		log:        logger,
	}

	mux := s.Handler()
	ts := httptest.NewServer(mux)

	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("GET failed: expected %d, got %d", http.StatusInternalServerError, resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("GET failed: expected %s, got %s", "application/json", resp.Header.Get("Content-Type"))
	}

	errMsg := ErrorMessage{}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("read body failed: %v", err)
	}

	err = json.Unmarshal(body, &errMsg)
	if err != nil {
		t.Errorf("unmarshal body failed: %v", err)
	}

	if errMsg.Error != "Error getting cluster info: cmon client error" {
		t.Errorf("wrong error message should be 'Error getting cluster info: cmon client error', got %s", errMsg.Error)
	}

	type log struct {
		Time  time.Time `json:"time"`
		Level string    `json:"level"`
		Msg   string    `json:"msg"`
		Error string    `json:"error"`
	}

	l := log{}

	err = json.Unmarshal(b.Bytes(), &l)
	if err != nil {
		t.Errorf("failed to unmarshal log: %v", err)
	}

	if l.Level != "ERROR" {
		t.Errorf("expected 'ERROR', got '%s'", l.Level)
	}

	if l.Msg != "Error getting cluster info" {
		t.Errorf("expected 'Error getting cluster info', got '%s'", l.Msg)
	}

	if l.Error != "cmon client error" {
		t.Errorf("expected 'cmon client error', got '%s'", l.Error)
	}
}

func TestHandlerIndexHandlerSuccess(t *testing.T) {
	cmonMock := CmonClientMock{}
	s := Service{
		cmonClient: cmonMock,
	}

	mux := s.Handler()
	ts := httptest.NewServer(mux)

	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET failed: expected status OK, got %v", resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("GET failed: expected Content-Type application/json, got %v", resp.Header.Get("Content-Type"))
	}

	var clusterTarget []ClusterTarget
	err = json.NewDecoder(resp.Body).Decode(&clusterTarget)
	if err != nil {
		t.Errorf("GET failed: %v", err)
	}

	expectedClusterTarget := []ClusterTarget{}
	expectedClusterTarget = append(expectedClusterTarget, ClusterTarget{
		Target: []string{"127.0.0.1:9011", "127.0.0.1:9100", "127.0.0.1:9104", "127.0.0.1:9215"},
		Label: map[string]string{
			"ClusterID":    "1",
			"ClusterName":  "cluster1",
			"ClusterType":  "postgresql_single",
			"ControllerId": "00000000-0000-0000-0000-000000000000",
			"cid":          "1",
			"tenant":       "acme",
			"tags":         "production,tenant:acme",
		},
	})

	if !reflect.DeepEqual(clusterTarget, expectedClusterTarget) {
		t.Errorf("GET failed: expected cluster target %v, got %v", expectedClusterTarget, clusterTarget)
	}
}

func TestAddTagLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		tags     []string
		expected map[string]string
	}{
		{
			name:     "no tags",
			labels:   map[string]string{"ClusterID": "1"},
			tags:     nil,
			expected: map[string]string{"ClusterID": "1"},
		},
		{
			name:   "key value and plain tags",
			labels: map[string]string{"ClusterID": "1"},
			tags:   []string{"tenant:acme", "env : prod", "production", "production"},
			expected: map[string]string{
				"ClusterID": "1",
				"tenant":    "acme",
				"env":       "prod",
				"tags":      "env:prod,production,tenant:acme",
			},
		},
		{
			name:   "equals sign is normalized to colon",
			labels: map[string]string{},
			tags:   []string{"tenant=acme", "env = prod"},
			expected: map[string]string{
				"tenant": "acme",
				"env":    "prod",
				"tags":   "env:prod,tenant:acme",
			},
		},
		{
			name:   "same tag with both separators is deduplicated",
			labels: map[string]string{},
			tags:   []string{"tenant=acme", "tenant:acme"},
			expected: map[string]string{
				"tenant": "acme",
				"tags":   "tenant:acme",
			},
		},
		{
			name:   "split on first separator",
			labels: map[string]string{},
			tags:   []string{"url:http://example.com", "query=a=b", "team:a=b"},
			expected: map[string]string{
				"url":   "http://example.com",
				"query": "a=b",
				"team":  "a=b",
				"tags":  "query:a=b,team:a=b,url:http://example.com",
			},
		},
		{
			name:   "invalid label names are sanitized",
			labels: map[string]string{},
			tags:   []string{"cost-center:42", "1st=yes", "a.b:c"},
			expected: map[string]string{
				"cost_center": "42",
				"_1st":        "yes",
				"a_b":         "c",
				"tags":        "1st:yes,a.b:c,cost-center:42",
			},
		},
		{
			name:   "existing and reserved labels are not overwritten",
			labels: map[string]string{"ClusterID": "1"},
			tags:   []string{"ClusterID:2", "__address__=evil", "tags:x", ":empty"},
			expected: map[string]string{
				"ClusterID": "1",
				"tags":      ":empty,ClusterID:2,__address__:evil,tags:x",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addTagLabels(tt.labels, tt.tags)
			if !reflect.DeepEqual(tt.labels, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, tt.labels)
			}
		})
	}
}
