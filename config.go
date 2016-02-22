// Copyright 2015 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"

	"github.com/la5nta/pat/cfg"
)

func LoadConfig(path string, fallback cfg.Config) (config cfg.Config, err error) {
	config, err = ReadConfig(path)
	if os.IsNotExist(err) {
		return fallback, WriteConfig(fallback, path)
	} else if err != nil {
		return config, err
	}

	// Ensure the alias "telnet" exists
	if config.ConnectAliases == nil {
		config.ConnectAliases = make(map[string]string)
	}
	if _, exists := config.ConnectAliases["telnet"]; !exists {
		config.ConnectAliases["telnet"] = cfg.DefaultConfig.ConnectAliases["telnet"]
	}

	return config, nil
}

func ReadConfig(path string) (config cfg.Config, err error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}

	err = json.Unmarshal(data, &config)
	return
}

func WriteConfig(config cfg.Config, filePath string) error {
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	// Add trailing new-line
	b = append(b, '\n')

	// Ensure path dir is available
	os.Mkdir(path.Dir(filePath), os.ModePerm|os.ModeDir)

	return ioutil.WriteFile(filePath, b, 0600)
}
