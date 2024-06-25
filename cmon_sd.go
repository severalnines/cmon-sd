// Copyright 2022 Severalnines
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"sort"
	"strconv"

	"flag"
	"fmt"

	"github.com/severalnines/cmon-proxy/cmon"
	"github.com/severalnines/cmon-proxy/cmon/api"
	"github.com/severalnines/cmon-proxy/config"
)

type ClusterTarget struct {
	Target []string          `json:"targets,omitempty"`
	Label  map[string]string `json:"labels,omitempty"`
}

type Cmon interface {
	Authenticate() error
	ControllerID() string
	GetAllClusterInfo(req *api.GetAllClusterInfoRequest) (*api.GetAllClusterInfoResponse, error)
}

type ErrorMessage struct {
	Error string `json:"error"`
}

type Service struct {
	cmonClient Cmon
	log        *slog.Logger
}

func NewService() (*Service, error) {
	cmonEndpoint := os.Getenv("CMON_ENDPOINT")
	cmonUsername := os.Getenv("CMON_USERNAME")
	cmonPassword := os.Getenv("CMON_PASSWORD")

	if cmonEndpoint == "" {
		cmonEndpoint = "https://127.0.0.1:9501"
	}

	if cmonUsername == "" {
		return nil, errors.New("CMON_USERNAME is required")
	}

	if cmonPassword == "" {
		return nil, errors.New("CMON_PASSWORD is required")
	}

	cmonClient := cmon.NewClient(&config.CmonInstance{
		Url:      cmonEndpoint,
		Username: cmonUsername,
		Password: cmonPassword,
	},
		30)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	return &Service{
		cmonClient: cmonClient,
		log:        logger,
	}, nil
}

func (s *Service) errorResponse(w http.ResponseWriter, statusCode int, message string) {
	m := ErrorMessage{Error: message}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(m)
}

func (s *Service) IndexHandler(w http.ResponseWriter, r *http.Request) {
	err := s.cmonClient.Authenticate()
	if err != nil {
		s.log.Error("Error authenticating", err)
		s.errorResponse(w, http.StatusUnauthorized, fmt.Sprintf("Error authenticating: %s", err.Error()))
		return
	}

	res, err := s.cmonClient.GetAllClusterInfo(&api.GetAllClusterInfoRequest{
		WithHosts: true,
	})

	if err != nil {
		s.log.Error("Error getting cluster info", "error", err)
		s.errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error getting cluster info: %s", err.Error()))
		return
	}

	clusterTarget := []ClusterTarget{}

	// iterate through all clusters
	for i, cluster := range res.Clusters {
		temp := ClusterTarget{
			Target: []string{},
			Label: map[string]string{
				"ClusterID":    strconv.FormatInt(int64(cluster.ClusterID), 10),
				"ClusterName":  cluster.ClusterName,
				"cid":          strconv.FormatInt(int64(cluster.ClusterID), 10),
				"ClusterType":  cluster.ClusterType,
				"ControllerId": s.cmonClient.ControllerID(),
			},
		}

		// iterate through all hosts for given cluster
		for _, host := range cluster.Hosts {
			switch host.Nodetype {
			case "controller", "prometheus", "keepalived":
				continue
			}

			// check host type and assign exporter port
			// node_exporter and process_exporter applies to any node type
			temp.Target = append(temp.Target, host.IP+":9100") // node exporter
			temp.Target = append(temp.Target, host.IP+":9011") // process exporter

			switch host.Nodetype {
			case "mysql", "galera":
				temp.Target = append(temp.Target, host.IP+":9104")
			case "haproxy":
				temp.Target = append(temp.Target, host.IP+":9600")
			case "mongo":
				switch host.Role {
				case "shardsvr":
					temp.Target = append(temp.Target, host.IP+":9216") // mongo exporter
				case "mongos":
					temp.Target = append(temp.Target, host.IP+":9215") // mongos exporter
				case "mongocfg":
					temp.Target = append(temp.Target, host.IP+":9214") // mongocfg exporter
				}
			case "mssql":
				temp.Target = append(temp.Target, host.IP+":9399")
			case "postgres":
				temp.Target = append(temp.Target, host.IP+":9187")
			case "redis":
				temp.Target = append(temp.Target, host.IP+":9121")
			case "proxysql":
				temp.Target = append(temp.Target, host.IP+":42004")
			case "pgbouncer":
				temp.Target = append(temp.Target, host.IP+":9127")
			}
		}

		sort.Strings(temp.Target)
		temp.Target = slices.Compact(temp.Target)
		clusterTarget = append(clusterTarget, temp)
		i++
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(clusterTarget)
}

func (s *Service) Handler() http.Handler {
	r := http.NewServeMux()
	r.HandleFunc("/", s.IndexHandler)

	return r
}

func main() {
	var port int

	flag.IntVar(&port, "p", 8080, "Listen port.")
	flag.Parse()

	service, err := NewService()
	if err != nil {
		log.Fatalf("Error creating handler: %v", err)
	}

	mux := service.Handler()
	listenAddress := fmt.Sprintf(":%d", port)
	log.Fatal(http.ListenAndServe(listenAddress, mux))
}
