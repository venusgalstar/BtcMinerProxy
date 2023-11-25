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
	"strconv"
	"time"
)

var numMiners, numUpstreams int
var avgHashrate float64

type FoundShare struct {
	Time time.Time
	Diff uint64
}

var foundShares = make([]FoundShare, 10)

func formatHashrate(f float64) string {
	if f > 1000*1000 {
		return strconv.FormatFloat(f/1000/1000, 'f', 1, 64) + " M"
	} else if f > 1000 {
		return strconv.FormatFloat(f/1000, 'f', 1, 64) + " k"
	} else {
		return strconv.FormatFloat(f, 'f', 0, 64) + " "
	}
}

type Hr struct {
	Hr     float64 `json:"hr"`
	Time   int64   `json:"time"`
	Miners int     `json:"miners"`
}

type UpstreamWorker struct {
	ID    string `json:"id"`
	Share struct {
		Accepted uint64 `json:"accepted"`
		Rejected uint64 `json:"rejected"`
	} `json:"shares"`
	Submit struct {
		Accepted uint64 `json:"accepted"`
		Rejected uint64 `json:"rejected"`
	} `json:"submits"`
}

type DownstreamWorker struct {
	ID    string `json:"id"`
	Share struct {
		Accepted uint64 `json:"accepted"`
		Stale    uint64 `json:"stale"`
		Invalid  uint64 `json:"rejected"`
	} `json:"shares"`
	Submit struct {
		Accepted uint64 `json:"accepted"`
		Stale    uint64 `json:"stale"`
		Invalid  uint64 `json:"rejected"`
	} `json:"submits"`
}

type UpstreamReport struct {
	Name      string           `json:"name"`
	Direction string           `json:"direction"`
	Workers   []UpstreamWorker `json:"workers"`
}

type DownstreamReport struct {
	Name      string             `json:"name"`
	Direction string             `json:"direction"`
	Workers   []DownstreamWorker `json:"workers"`
}

type Report struct {
	Timestamp string `json:"timestamp"`
	Streams   struct {
		Upstreams   []UpstreamReport
		Downstreams []DownstreamReport
	} `json:"streams"`
}

var globalReport *Report
var hrChart = make([]Hr, 0, 288)

func Stats() {

	refreshReport()
	globalReport = &Report{}

	go func() {
		for {
			time.Sleep(5 * time.Minute)

			getStats()

			if len(hrChart) == 288 {
				hrChart = hrChart[1:]
			}

			hrChart = append(hrChart, Hr{
				Hr:     avgHashrate,
				Time:   time.Now().Unix(),
				Miners: numMiners,
			})
		}
	}()

	for {
		getStats()
		makeReport()
		venuslog.Statsf("%s avg, miners: "+venuslog.COLOR_CYAN+"%d"+venuslog.COLOR_WHITE+", upstreams: "+venuslog.COLOR_CYAN+"%d"+venuslog.COLOR_WHITE,
			venuslog.COLOR_CYAN+formatHashrate(avgHashrate)+"H/s"+venuslog.COLOR_WHITE,
			numMiners,
			numUpstreams,
		)
		time.Sleep(time.Duration(config.CFG.PrintInterval) * time.Second)
	}
}

func makeReport() {

	reportStr, _ := json.Marshal(globalReport)
	writeReport(string(reportStr[:]))

	globalReport = &Report{}
	t := time.Now()
	globalReport.Timestamp = t.String()
	venuslog.Warn("Timestamp", globalReport.Timestamp)

	for _, upstream := range Upstreams {

		uReport := &UpstreamReport{}
		uReport.Name = config.CFG.Pools[upstream.server.PoolId].Url
		uReport.Direction = "upstream"
		uWorker := &UpstreamWorker{}
		uWorker.ID = config.CFG.Pools[upstream.server.PoolId].User

		uWorker.Share.Accepted = upstream.Shares.Accepted
		uWorker.Share.Rejected = upstream.Shares.Rejected

		uWorker.Submit.Accepted = upstream.Submits.Accepted
		uWorker.Submit.Rejected = upstream.Submits.Rejected

		uReport.Workers = append(uReport.Workers, *uWorker)
		globalReport.Streams.Upstreams = append(globalReport.Streams.Upstreams, *uReport)

		dReport := &DownstreamReport{}
		dReport.Name = upstream.server.Conn.RemoteAddr().String()
		dReport.Direction = "downstream"
		dWorker := &DownstreamWorker{}
		dWorker.ID = upstream.server.WorkerID

		dWorker.Share.Accepted = upstream.server.Shares.Accepted
		dWorker.Share.Invalid = upstream.server.Shares.Invalid
		dWorker.Share.Stale = upstream.server.Shares.Stale
		dWorker.Submit.Accepted = upstream.server.Submits.Accepted
		dWorker.Submit.Invalid = upstream.server.Submits.Invalid
		dWorker.Submit.Stale = upstream.server.Submits.Stale

		dReport.Workers = append(dReport.Workers, *dWorker)

		globalReport.Streams.Downstreams = append(globalReport.Streams.Downstreams, *dReport)

	}
	//Added for report

}

func getStats() {
	shares2 := make([]FoundShare, 0, len(foundShares))
	var totalDiff float64

	for _, v := range foundShares {
		if time.Since(v.Time) <= config.HASHRATE_AVG_MINUTES*time.Minute {
			shares2 = append(shares2, v)
			totalDiff += float64(v.Diff)
		}
	}
	foundShares = shares2
	avgHashrate = totalDiff / (config.HASHRATE_AVG_MINUTES * 60)

	// TODO

	//srv.ConnsMut.Lock()
	numMiners = len(srv.Connections)
	//srv.ConnsMut.Unlock()

	//UpstreamsMut.Lock()
	numUpstreams = len(Upstreams)
	//UpstreamsMut.Unlock()
}
