/*
 * BtcMinerProxy is a high-performance Cryptonote Stratum mining proxy.
 * Copyright (C) 2023 Venusgalstar
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package main

import (
	"btcminerproxy/config"
	"btcminerproxy/stats"
	"btcminerproxy/venuslog"
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// This is main file of this project where main function is definited.
// Main Function does actions as listed
// 1. Load and parsing configuration parameter from config.json
// 2. Start web server for api
// 3. Start proxy process

func main() {

	// Load configuration parameters from config.json as json format
	err := loadConfig()

	if err != nil {
		venuslog.Info(fmt.Sprintf("Failed to read config.json (%s), running configurator", err))
		configurator()
	}

	err = config.CFG.Validate()
	if err != nil {
		venuslog.Fatal(err)
	}

	// Connecting to redis
	errDB := connectRedis()

	if errDB != nil {
		venuslog.Fatal("Failed to connect to database", errDB)
	}

	// After checking loading info
	venuslog.StartLogger()

	threads := runtime.GOMAXPROCS(0)
	if threads > config.CFG.MaxConcurrency {
		threads = config.CFG.MaxConcurrency
		runtime.GOMAXPROCS(config.CFG.MaxConcurrency)
	}

	// Start webserver for api
	go StartDashboard()

	// Set log information
	if config.CFG.Title {
		colCyan := venuslog.COLOR_CYAN
		colGreen := venuslog.COLOR_GREEN
		colWhite := venuslog.COLOR_WHITE
		bold := venuslog.BOLD

		numThreads := strconv.FormatInt(int64(threads), 10)
		threadsCol := venuslog.COLOR_GREEN

		if numThreads == "2" {
			threadsCol = venuslog.COLOR_YELLOW
		} else if numThreads == "1" {
			threadsCol = venuslog.COLOR_RED
		}

		hasCgo := "cgo"
		if !stats.CGO {
			hasCgo = ""
		}

		venuslog.Printf("%s * %s%s\n", bold+colGreen, colWhite,
			"VERSION      "+colCyan+"BtcMinerProxy"+colWhite+" v"+config.VERSION.ToString())
		venuslog.Printf("%s * %s%s\n", bold+colGreen, colWhite,
			"CREDITS      "+colCyan+"Developed by "+colWhite+"Venusgalstar"+colCyan+".")
		venuslog.Printf("%s * %s%s\n", bold+colGreen, colWhite,
			"PLATFORM     "+runtime.GOOS+"/"+runtime.GOARCH+" "+venuslog.COLOR_CYAN+hasCgo)
		venuslog.Printf("%s * %s%s\n", bold+colGreen, colWhite,
			"CONCURRENCY  "+threadsCol+numThreads+colWhite+" threads")

		for i, v := range config.CFG.Pools {
			col := colCyan
			if v.Tls {
				col = colGreen
			}

			venuslog.Printf("%s * %s%s\n", bold+colGreen, colWhite,
				fmt.Sprintf("POOL #%d      %s", i, col+v.Url+venuslog.COLOR_RESET))
		}

	}

	if config.CFG.Dashboard.Enabled {
		venuslog.Info(fmt.Sprintf("Dashboard is available at http://127.0.0.1:%d", config.CFG.Dashboard.Port))
	}

	venuslog.Info("Using pool", config.CFG.Pools[config.CFG.PoolIndex].Url)

	// Start stats process for monitoring
	go Stats()

	// Start main proxy process
	StartProxy()
}

// Load configuration parameters from json
func loadConfig() error {
	data, err := os.ReadFile("./config.json")
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &config.CFG)
}

var wordRegexp = regexp.MustCompile("^\\w+$")

// Load configuration parameters from json
func configurator() {
	userAddr := prompt("Enter your wallet address: ")
	venuslog.Info(userAddr)

	addr := []byte(userAddr)

	if !wordRegexp.Match([]byte(addr)) {
		venuslog.Fatal("Invalid address", addr)
	}
	curcfg := strings.ReplaceAll(string(config.DefaultConfig), "YOUR_WALLET_ADDRESS", userAddr)

	curcfg = strings.ReplaceAll(curcfg, "PORT_TLS", "3334")
	curcfg = strings.ReplaceAll(curcfg, "PORT_NO_TLS", "3333")

	os.WriteFile("./config.json", []byte(curcfg), 0o666)
	err := json.Unmarshal([]byte(curcfg), &config.CFG)
	if err != nil {
		venuslog.Fatal(err)
	}
}

func prompt(lbl string) string {
	var str string
	r := bufio.NewReader(os.Stdin)
	for {
		fmt.Print(lbl)
		str, _ = r.ReadString('\n')
		if str != "" {
			break
		}
	}
	return strings.TrimSpace(str)
}
