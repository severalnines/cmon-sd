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

	return &api.GetAllClusterInfoResponse{
		Clusters: []*api.Cluster{
			{
				ClusterID:   1,
				ClusterName: "cluster1",
				ClusterType: "postgresql_single",
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
		},
	})

	if !reflect.DeepEqual(clusterTarget, expectedClusterTarget) {
		t.Errorf("GET failed: expected cluster target %v, got %v", expectedClusterTarget, clusterTarget)
	}
}
