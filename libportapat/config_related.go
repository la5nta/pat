package libportapat

import (
	"encoding/json"
	"errors"
	"fmt"
	cfg2 "github.com/la5nta/pat/cfg"
	"github.com/la5nta/pat/libportapat/porta_paths"
	"log"
	"math/rand"
	"os"
	"strings"
)

func SetRandomizePort(rndport bool) {
	flagRandomisePort = rndport
}

func doRandomisePort() error {
	newPort := 48000 + rand.Intn(1000)
	strConfig := ConfigRead()
	var cfg cfg2.Config

	if len(strConfig) < 1 || strings.Contains(strConfig, "!!! ERROR READING CONFIG") {
		cfg = cfg2.DefaultConfig
	} else {
		err := json.Unmarshal([]byte(strConfig), &cfg)
		if err != nil {
			log.Println("Error unmarshalling: ", err)
			cfg = cfg2.DefaultConfig
		}
	}

	cfg.HTTPAddr = fmt.Sprintf("127.0.0.1:%d", newPort)

	bytConfig, err := json.MarshalIndent(&cfg, "", "\t")
	if err != nil {
		return err
	}
	return ConfigWrite(string(bytConfig))
}

func ConfigRead() string {
	cfgPath := porta_paths.GetPortaPath(porta_paths.PATH_KIND_CONFIG)
	cfgBinary, err := os.ReadFile(cfgPath)
	if err != nil {
		fmt.Printf("Error reading config file: %s\n", err)
		return "!!! ERROR READING CONFIG: " + err.Error()
	}
	return string(cfgBinary)
}

func ConfigWrite(newconf string) error {
	var cfg cfg2.Config
	errc := json.Unmarshal([]byte(newconf), &cfg)
	if errc != nil {
		return errors.New("Format error: " + errc.Error())
	}

	cfgPath := porta_paths.GetPortaPath(porta_paths.PATH_KIND_CONFIG)
	err := os.WriteFile(cfgPath, []byte(newconf), 0644)
	if err != nil {
		fmt.Printf("Error writing config file: %s\n", err)
	}
	return nil
}
