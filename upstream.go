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
	"btcminerproxy/mutex"
	stratumclient "btcminerproxy/stratum/client"
	"btcminerproxy/stratum/rpc"
	stratumserver "btcminerproxy/stratum/server"
	"btcminerproxy/stratum/template"
	"btcminerproxy/venuslog"
	"bufio"
	"time"
)

type Upstream struct {
	ID     uint64
	client *stratumclient.Client
	server *stratumserver.Connection
}

var Upstreams = make(map[uint64]*Upstream, 100)
var UpstreamsMut mutex.Mutex
var LatestUpstream uint64

// Send Subscribe to Pool
func SendSubscribe(conn *stratumserver.Connection, data []byte) {

	if conn.Upstream != 0 {
		venuslog.Warn("Already connected")
		return
	}

	newId := LatestUpstream + 1
	client := &stratumclient.Client{}

	err := client.SendSubscribe(config.CFG.Pools[conn.PoolId].Url, data, newId)

	if err != nil {
		venuslog.Warn("Error while sending subscribe to pool")
		Kick(newId)
	}

	Upstreams[newId] = &Upstream{
		ID:     newId,
		client: client,
		server: conn,
	}
	conn.Upstream = newId

	go handleDownstream(newId)
}

func handleDownstream(upstreamId uint64) {

	cl := Upstreams[upstreamId].client

	for {
		response := &template.StratumMsg{}
		cl.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		reader := bufio.NewReaderSize(cl.Conn, config.MAX_REQUEST_SIZE)
		data, isPrefix, errR := reader.ReadLine()

		venuslog.Warn("Received data from pool")

		if errR != nil || isPrefix {
			venuslog.Warn("ReadJSON failed in proxy:", errR)
			return
		}

		err := rpc.ReadJSON(&response, data)

		if err != nil {
			venuslog.Warn("ReadJSON failed in proxy as server response:", err)
		}

		// severMsg := &template.StratumSeverMsg{}

		// err1 := rpc.ReadJSON(&severMsg, data)

		// if err1 != nil {
		// 	venuslog.Warn("ReadJSON failed in proxy as server msg:", err1)
		// }

		Upstreams[upstreamId].server.SendBytes(data)
	}
}

// upstream must be locked before closing
func (us *Upstream) Close() {

	us.client.Close()

	Upstreams[us.ID] = nil

	if LatestUpstream == us.ID {
		if len(Upstreams) == 1 {
			venuslog.Debug("Last upstream destroyed.")
			LatestUpstream = 0
		}
	}

	delete(Upstreams, us.ID)
}
