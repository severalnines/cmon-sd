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
	"log"
	"net/http"
	"os"
	"strconv"

	"flag"
	"fmt"
	"github.com/severalnines/cmon-proxy/cmon"
	"github.com/severalnines/cmon-proxy/cmon/api"
	"github.com/severalnines/cmon-proxy/config"
)

const namespace = "cmon"

var cmonEndpoint string
var cmonUsername string
var cmonPassword string

type ClusterTarget struct {
	Target []string          `json:"targets,omitempty"`
	Label  map[string]string `json:"labels,omitempty"`
}

func IndexHandler(w http.ResponseWriter, r *http.Request) {

	client := cmon.NewClient(&config.CmonInstance{
		Url:      cmonEndpoint,
		Username: cmonUsername,
		Password: cmonPassword,
	},
		30)

	err := client.Authenticate()
	if err != nil {
		res, err := client.Ping()
		log.Println("Test: ", err, res)
		return
	}

	res, err := client.GetAllClusterInfo(&api.GetAllClusterInfoRequest{
		WithHosts: true,
	})

	if err != nil {
		log.Println("Test: ", err, res)
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
				"ControllerId": client.ControllerID(),
			},
		}

		// iterate through all hosts for given cluster
		for _, host := range cluster.Hosts {

			if host.Nodetype == "controller" {
				continue
			}

			if host.Nodetype == "prometheus" {
				continue
			}

			if host.Nodetype == "keepalived" {
				continue
			}

			//check host type and assign exporter port
			// node_exporter and process_exporter applies to any node type
			temp.Target = append(temp.Target, host.IP+":9100") // node exporter
			temp.Target = append(temp.Target, host.IP+":9011") // process exporter

			if host.Nodetype == "mysql" || host.Nodetype == "galera" {

				temp.Target = append(temp.Target, host.IP+":9104") // mysql exporter
			}

			if host.Nodetype == "haproxy" {
				temp.Target = append(temp.Target, host.IP+":9600") // haproxy exporter
			}

			if host.Nodetype == "mongo" {
				if host.Role == "shardsvr" {
					temp.Target = append(temp.Target, host.IP+":9216") // mongo exporter
				}
				if host.Role == "mongos" {
					temp.Target = append(temp.Target, host.IP+":9215") // mongos exporter
				}

				if host.Role == "mongocfg" {
					temp.Target = append(temp.Target, host.IP+":9214") // mongocfg exporter
				}
			}

			if host.Nodetype == "mssql" {
				temp.Target = append(temp.Target, host.IP+":9399") // mssql exporter
			}

			if host.Nodetype == "postgres" {
				temp.Target = append(temp.Target, host.IP+":9187") // postgres exporter
			}

			if host.Nodetype == "redis" {
				temp.Target = append(temp.Target, host.IP+":9121") // redis exporter
			}

			if host.Nodetype == "proxysql" {
				temp.Target = append(temp.Target, host.IP+":42004") // proxysql exporter
			}

			if host.Nodetype == "pgbouncer" {
				temp.Target = append(temp.Target, host.IP+":9127") // pgbouncer exporter
			}

		}
		temp.Target = removeDuplicateStr(temp.Target)
		clusterTarget = append(clusterTarget, temp)
		i++
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(clusterTarget)
}

func removeDuplicateStr(strSlice []string) []string {
	allKeys := make(map[string]bool)
	list := []string{}
	for _, item := range strSlice {
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}
	return list
}

var port int

func init() {
	flag.IntVar(&port, "p", 8080, "Listen port.")
	flag.Parse()
}

func main() {
	cmonEndpoint = os.Getenv("CMON_ENDPOINT")
	cmonUsername = os.Getenv("CMON_USERNAME")
	cmonPassword = os.Getenv("CMON_PASSWORD")

	if cmonEndpoint == "" {
		cmonEndpoint = "https://127.0.0.1:9501"
	}

	if cmonUsername == "" {
		log.Fatalf("Env variable CMON_USERNAME is not set.")
	}

	if cmonPassword == "" {
		log.Fatalf("Env variable CMON_PASSWORD is not set.")
	}

	http.HandleFunc("/", IndexHandler)
	listenAddress := fmt.Sprintf(":%d", port)
	log.Fatal(http.ListenAndServe(listenAddress, nil))
}
