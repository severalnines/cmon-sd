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
        "ClusterID":   strconv.FormatInt(int64(cluster.ClusterID), 10),
        "ClusterName": cluster.ClusterName,
        "cid":         strconv.FormatInt(int64(cluster.ClusterID), 10),
        "ClusterType": cluster.ClusterType,
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

      //check host type and assign exporter port
      temp.Target = append(temp.Target, host.IP+":9100") // node exporter
      temp.Target = append(temp.Target, host.IP+":9011") // process exporter

      if host.Nodetype == "mysql" {
        temp.Target = append(temp.Target, host.IP+":9104") // mysql exporter
      }

    }

    clusterTarget = append(clusterTarget, temp)
    i++
  }

  w.Header().Set("Content-Type", "application/json")
  w.WriteHeader(http.StatusOK)
  json.NewEncoder(w).Encode(clusterTarget)

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
  log.Fatal(http.ListenAndServe(":8080", nil))
}
