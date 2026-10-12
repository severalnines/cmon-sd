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
	"strings"

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

	err := cmonClient.Authenticate()
	if err != nil {
		return nil, err
	}

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
		s.log.Error("Error authenticating", "error",  err)
		s.errorResponse(w, http.StatusUnauthorized, fmt.Sprintf("Error authenticating: %s", err.Error()))
		return
	}

	res, err := s.cmonClient.GetAllClusterInfo(&api.GetAllClusterInfoRequest{
		WithHosts: true,
		WithTags:  true,
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
		addTagLabels(temp.Label, cluster.Tags)

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
			case "valkey":
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

// tagsLabel holds all cluster tags joined as ",tag1,tag2," (sorted) so that
// any tag can be matched in Prometheus relabeling with a regex like ".*,foo,.*".
const tagsLabel = "tags"

// addTagLabels adds cluster tags to labels. A tag of the form "key:value" or
// "key=value" becomes the label key="value". The tag is split on the first
// ":" or "=", so the value may contain either character (e.g.
// "url:http://host"). In the combined tags label, key/value tags are
// normalized to "key:value". The key is sanitized to a valid Prometheus label
// name. Existing labels are never overwritten, and keys starting with "__"
// are skipped since they are reserved by Prometheus.
func addTagLabels(labels map[string]string, tags []string) {
	if len(tags) == 0 {
		return
	}

	type keyValue struct{ key, value string }
	normalized := make([]string, 0, len(tags))
	pairs := map[string]keyValue{}

	for _, tag := range tags {
		sep := strings.IndexAny(tag, ":=")
		if sep < 0 {
			normalized = append(normalized, tag)
			continue
		}

		key, value := strings.TrimSpace(tag[:sep]), strings.TrimSpace(tag[sep+1:])
		tag = key + ":" + value
		normalized = append(normalized, tag)
		pairs[tag] = keyValue{key, value}
	}

	sort.Strings(normalized)
	normalized = slices.Compact(normalized)

	if _, exists := labels[tagsLabel]; !exists {
		labels[tagsLabel] = "," + strings.Join(normalized, ",") + ","
	}

	for _, tag := range normalized {
		kv, ok := pairs[tag]
		if !ok {
			continue
		}

		key := sanitizeLabelName(kv.key)
		if key == "" || strings.HasPrefix(key, "__") {
			continue
		}

		if _, exists := labels[key]; exists {
			continue
		}

		labels[key] = kv.value
	}
}

// sanitizeLabelName replaces characters not allowed in a Prometheus label
// name ([a-zA-Z_][a-zA-Z0-9_]*) with underscores, and prefixes names that
// start with a digit with an underscore.
func sanitizeLabelName(name string) string {
	if name == "" {
		return ""
	}

	b := []byte(name)
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
		default:
			b[i] = '_'
		}
	}

	if b[0] >= '0' && b[0] <= '9' {
		return "_" + string(b)
	}

	return string(b)
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

	if portStr := os.Getenv("CMON_SD_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}

	service, err := NewService()
	if err != nil {
		log.Fatalf("Error creating the service: %v", err)
	}

	mux := service.Handler()
	listenAddress := fmt.Sprintf(":%d", port)
	log.Fatal(http.ListenAndServe(listenAddress, mux))
}
