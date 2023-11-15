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
	stratumserver "btcminerproxy/stratum/server"
	"btcminerproxy/venuslog"
	"io"
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
		Kick(conn.Id)
	}

	Upstreams[newId] = &Upstream{
		ID:     newId,
		client: client,
		server: conn,
	}
	conn.Upstream = newId

	venuslog.Warn("New upstream id ", newId)

	go handleDownstream(newId)
}

func handleDownstream(upstreamId uint64) {

	cl := Upstreams[upstreamId].client
	buf := make([]byte, config.MAX_REQUEST_SIZE)

	for {
		cl.Conn.SetReadDeadline(time.Now().Add(config.READ_TIMEOUT_SECONDS * time.Second))
		nr, err := cl.Conn.Read(buf)

		if err != nil {
			if err == io.EOF {
				continue
			}
			venuslog.Warn("ReadJSON failed in proxy from pool:", err)
			return
		}

		venuslog.Warn("Received data from pool")
		str := string(buf[:])
		venuslog.Warn("data:", str)

		nw, nerr := Upstreams[upstreamId].server.Conn.Write(buf[0:nr])

		if nerr != nil {
			venuslog.Warn("err on write ", nerr)
		}

		venuslog.Warn("read bytes", nr)
		venuslog.Warn("write bytes ", nw)
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
