package main

import (
    "fmt"
    "github.com/ethpandaops/pandafuzz/pkg/common"
    "gopkg.in/yaml.v2"
    "io/ioutil"
)

func main() {
    data, _ := ioutil.ReadFile("master.yaml")
    var config struct {
        Master common.MasterConfig `yaml:"master"`
    }
    yaml.Unmarshal(data, &config)
    fmt.Printf("Database path from config: %s\n", config.Master.Database.Path)
}
