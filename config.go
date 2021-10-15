// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/la5nta/pat/cfg"
)

func LoadConfig(cfgPath string, fallback cfg.Config) (config cfg.Config, err error) {
	config, err = ReadConfig(cfgPath)
	if os.IsNotExist(err) {
		return fallback, WriteConfig(fallback, cfgPath)
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

	// Ensure ServiceCodes has a default value
	if len(config.ServiceCodes) == 0 {
		config.ServiceCodes = cfg.DefaultConfig.ServiceCodes
	}

	// Ensure Pactor has a default value
	if config.Pactor == (cfg.PactorConfig{}) {
		config.Pactor = cfg.DefaultConfig.Pactor
	}

	// TODO: Remove after some release cycles (2019-09-29)
	if config.GPSdAddrLegacy != "" {
		config.GPSd.Addr = config.GPSdAddrLegacy
	}

	return config, nil
}

func replaceDeprecatedCMSHostname(path string, data []byte) ([]byte, error) {
	const o = "@server.winlink.org:8772/wl2k"
	const n = "@cms.winlink.org:8772/wl2k"

	if !bytes.Contains(data, []byte(o)) {
		return data, nil
	}

	data = bytes.ReplaceAll(data, []byte(o), []byte(n))

	f, err := os.Open(path)
	if err != nil {
		return data, err
	}
	stat, err := f.Stat()
	f.Close()
	if err != nil {
		return data, err
	}
	return data, os.WriteFile(path, data, stat.Mode())
}

func ReadConfig(path string) (config cfg.Config, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	// TODO: Remove after some release cycles (2017-11-09)
	data, err = replaceDeprecatedCMSHostname(path, data)
	if err != nil {
		fmt.Println("Failed to rewrite deprecated CMS hostname:", err)
		fmt.Println("Please update your config's 'telnet' connect alias manually to:")
		fmt.Println(cfg.DefaultConfig.ConnectAliases["telnet"])
		fmt.Println("")
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

	return os.WriteFile(filePath, b, 0o600)
}
