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
	"net"
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

// Create new upstream for incomming connection from miner
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

// Sending mining.subscribe msg of stratum to mining pool
func SendSubscribe(conn *stratumserver.Connection, data []byte) {

	if conn.Upstream == 0 {
		CreateNewUpstream(conn)
	}

	err := Upstreams[conn.Upstream].client.SendData(data)

	if err != nil {
		venuslog.Warn("Error while sending configure to pool")
	}
}

// Sending mining.configure msg of stratum to mining pool
func SendConfigure(conn *stratumserver.Connection, data []byte) {

	if conn.Upstream == 0 {
		CreateNewUpstream(conn)
	}

	err := Upstreams[conn.Upstream].client.SendData(data)

	if err != nil {
		venuslog.Warn("Error while sending configure to pool")
	}
}

// Sending data of stratum to mining pool
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

// Handling downstreaming data from mining pool to miner
func handleDownstream(upstreamId uint64) {

	cl := Upstreams[upstreamId].client
	totalBuf := make([]byte, config.MAX_REQUEST_SIZE)
	bufLen := 0
	cl.Conn.SetReadDeadline(time.Now().Add(config.READ_TIMEOUT_SECONDS * time.Second))

	for {
		msg, msgLen, readLen, err := template.ReadLineFromSocket(cl.Conn, totalBuf, bufLen)

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

		// totalBuf = totalBuf[msgLen+1:]
		bufLen = bufLen + readLen - msgLen - 1

		venuslog.Warn("Received data from pool")

		str := string(msg[:])
		venuslog.Warn("data from upstream:", str)

		req := template.StratumMsg{}
		errJson := rpc.ReadJSON(&req, msg)

		if errJson != nil {
			venuslog.Warn("ReadJSON failed in proxy from miner:", errJson)
			UpstreamsMut.Lock()
			Upstreams[upstreamId].Close()
			UpstreamsMut.Unlock()
			return
		}

		_, nerr := Upstreams[upstreamId].server.Conn.Write(append(msg, '\n'))

		if nerr != nil {
			venuslog.Warn("err on write ", nerr)
			UpstreamsMut.Lock()
			Upstreams[upstreamId].Close()
			UpstreamsMut.Unlock()
			return
		}

		copy(totalBuf, totalBuf[msgLen+1:])

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

// disconnect miner
func disconnectMiner(remoteAddr string) (err error) {

	venuslog.Warn("trying to delete miner", remoteAddr)

	for _, upstream := range Upstreams {

		venuslog.Warn("Deleted miner", upstream.server.Conn.RemoteAddr().String())
		host, _, _ := net.SplitHostPort(upstream.server.Conn.RemoteAddr().String())

		if host != remoteAddr {
			continue
		}

		upstream.Close()

		venuslog.Warn("Deleted miner", remoteAddr)

	}
	return nil
}
