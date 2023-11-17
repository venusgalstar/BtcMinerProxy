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

func CreateNewUpstream(conn *stratumserver.Connection) {

	venuslog.Warn("Trying to create new upstream")

	UpstreamsMut.Lock()

	newId := LatestUpstream + 1
	client := &stratumclient.Client{}

	err := client.Connect(config.CFG.Pools[conn.PoolId].Url, newId)

	if err != nil {
		venuslog.Warn("Error while sending connecting to pool")
		UpstreamsMut.Unlock()
		Kick(conn.Id)
		return
	}

	Upstreams[newId] = &Upstream{
		ID:     newId,
		client: client,
		server: conn,
	}
	conn.Upstream = newId

	UpstreamsMut.Unlock()

	go handleDownstream(newId)

	venuslog.Warn("New upstream id ", newId)
}

func SendSubscribe(conn *stratumserver.Connection, data []byte) {

	if conn.Upstream == 0 {
		CreateNewUpstream(conn)
	}

	err := Upstreams[conn.Upstream].client.SendData(data)

	if err != nil {
		venuslog.Warn("Error while sending configure to pool")
	}
}

func SendConfigure(conn *stratumserver.Connection, data []byte) {

	if conn.Upstream == 0 {
		CreateNewUpstream(conn)
	}

	err := Upstreams[conn.Upstream].client.SendData(data)

	if err != nil {
		venuslog.Warn("Error while sending configure to pool")
	}
}

func SendData(conn *stratumserver.Connection, data []byte) {

	if conn.Upstream == 0 {
		venuslog.Warn("Connection broken")
		Kick(conn.Id)
		return
	}

	err := Upstreams[conn.Upstream].client.SendData(data)

	if err != nil {
		venuslog.Warn("Connection broken")
		Kick(conn.Id)
	}
}

func handleDownstream(upstreamId uint64) {

	cl := Upstreams[upstreamId].client
	buf := make([]byte, config.MAX_REQUEST_SIZE)
	bufLen := 0
	cl.Conn.SetReadDeadline(time.Now().Add(config.READ_TIMEOUT_SECONDS * time.Second))

	for {
		msg, msgLen, readLen, err := template.ReadLineFromSocket(cl.Conn, buf, bufLen)

		if err != nil || msgLen == 0 {
			if err == io.EOF || msgLen == 0 {
				continue
			}
			venuslog.Warn("Read failed in proxy from pool socket:", err)
			UpstreamsMut.Lock()
			Upstreams[upstreamId].Close()
			UpstreamsMut.Unlock()
			return
		}

		buf = buf[msgLen+1:]
		bufLen = bufLen + readLen - msgLen - 1

		venuslog.Warn("Received data from pool")

		req := template.StratumMsg{}
		errJson := rpc.ReadJSON(&req, msg)

		if errJson != nil {
			venuslog.Warn("ReadJSON failed in proxy from miner:", errJson)
			UpstreamsMut.Lock()
			Upstreams[upstreamId].Close()
			UpstreamsMut.Unlock()
			return
		}

		str := string(msg[:])
		venuslog.Warn("data:", str)

		_, nerr := Upstreams[upstreamId].server.Conn.Write(append(msg, '\n'))

		if nerr != nil {
			venuslog.Warn("err on write ", nerr)
			UpstreamsMut.Lock()
			Upstreams[upstreamId].Close()
			UpstreamsMut.Unlock()
			return
		}

	}
}

// upstream must be locked before closing
func (us *Upstream) Close() {

	us.client.Close()
	us.server.Close()

	Upstreams[us.ID] = nil

	if LatestUpstream == us.ID {
		if len(Upstreams) == 1 {
			venuslog.Debug("Last upstream destroyed.")
			LatestUpstream = 0
		}
	}

	delete(Upstreams, us.ID)
}
