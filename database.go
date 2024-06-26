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
	"btcminerproxy/venuslog"
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-redis/redis"
)

var db *redis.Client
var whiteList = make(map[string]bool, 100)
var blackList = make(map[string]bool, 100)

// Connecting to redis server
func connectRedis() error {

	venuslog.Warn("redis", os.Getenv("REDIS_DB_URL"))

	db = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_DB_URL"),
		Password: "",
		DB:       0,
	})

	pong, err := db.Ping().Result()

	venuslog.Warn("Database connecting result is ", pong)

	whiteResult, err1 := db.Get("whitelist").Result()

	if err1 != nil {
		venuslog.Warn("error while reading whitelist", err1)
	}

	json.Unmarshal([]byte(whiteResult), &whiteList)

	blackResult, err2 := db.Get("blacklist").Result()
	if err2 != nil {
		venuslog.Warn("error while reading blacklist", err2)
	}
	json.Unmarshal([]byte(blackResult), &blackResult)

	return err
}

func addList(remoteAddr string, isWhite bool) {

	if isWhite {
		whiteList[remoteAddr] = true
	} else {
		blackList[remoteAddr] = true
	}

	whitestr, _ := json.Marshal(whiteList)
	blackstr, _ := json.Marshal(blackList)

	db.Set("whitestr", string(whitestr[:]), 0)
	db.Set("blackstr", string(blackstr[:]), 0)

	venuslog.Warn("whitestr", string(whitestr[:]))
	venuslog.Warn("blackstr", string(blackstr[:]))
}

func delList(remoteAddr string, isWhite bool) {

	if isWhite {
		delete(whiteList, remoteAddr)
	} else {
		delete(blackList, remoteAddr)
	}

	whitestr, _ := json.Marshal(whiteList)
	blackstr, _ := json.Marshal(blackList)

	db.Set("whitestr", string(whitestr[:]), 0)
	db.Set("blackstr", string(blackstr[:]), 0)

	venuslog.Warn("whitestr", string(whitestr[:]))
	venuslog.Warn("blackstr", string(blackstr[:]))
}

func getList(isWhite bool) map[string]bool {

	if isWhite {
		// whitestr, _ := json.Marshal(whiteList)
		return whiteList
	}

	// blackstr, _ := json.Marshal(blackList)
	return blackList
}

func setPool(poolUrlStr string, minerIpStr string) string {

	var foundPool = -1
	for idxPool, pool := range config.CFG.Pools {

		if pool.Url == poolUrlStr {
			foundPool = idxPool
			break
		}
	}

	if foundPool == -1 {
		return string("Not found pool with url:")
	}

	var foundMiner = -1
	for idxMiner, miner := range config.CFG.Miners {

		if miner.IP == minerIpStr {
			foundMiner = idxMiner
			break
		}
	}

	if foundMiner == -1 {
		return string("Not found miner with ip:")
	}

	closeAllUpstreamFromMiner(minerIpStr)

	config.CFG.Miners[foundMiner].PoolUrl = poolUrlStr

	return string("switched pool")
}

func showPools() string {
	var globalPoolStatus []*PoolRatingHash

	for _, upstream := range Upstreams {

		var sumRatingHash = uint64(0)

		for _, job := range upstream.Jobs {
			sumRatingHash += job.Difficulty
		}

		poolStatus := &PoolRatingHash{}
		poolStatus.RatingHash = sumRatingHash
		poolStatus.PoolUrl = config.CFG.Pools[upstream.server.PoolId].Url
		globalPoolStatus = append(globalPoolStatus, poolStatus)
	}

	currentStatus, _ := json.Marshal(globalPoolStatus)
	accStatus, _ := json.Marshal(globalPoolInfo)
	message := fmt.Sprintf("current:{%s}, total:{%s}", currentStatus, accStatus)

	return message

}

func writeReport(reportStr string) string {

	db.Set("report", reportStr, 0)

	return string("ok")
}

func getReport() string {
	oldLog, _ := db.Get("report").Result()
	return oldLog
}

func refreshReport() {
	db.Set("report", "", 0)
}

func checkBlackList(ipAddr string) bool {
	return blackList[ipAddr]
}
