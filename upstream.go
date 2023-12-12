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
	"strconv"
	"strings"
	"time"
)

type Job struct {
	Difficulty uint64
	Status     uint64
}

type Upstream struct {
	ID     uint64
	client *stratumclient.Client
	server *stratumserver.Connection

	// added for report
	Shares struct {
		Accepted uint64
		Rejected uint64
	}
	Submits struct {
		Accepted uint64
		Rejected uint64
	}

	// checking jobs
	Jobs map[uint64]*Job
}

var Upstreams = make(map[uint64]*Upstream, 100)
var UpstreamsMut mutex.Mutex
var LatestUpstream uint64

func findPoolUrl(minerIp string) (string, uint64) {

	var poolIndex uint64 = 0
	poolUrl := ""

	for _, miner := range config.CFG.Miners {

		if miner.IP != minerIp {
			continue
		}

		poolUrl = miner.PoolUrl

		break
	}

	if poolUrl == "" {
		poolUrl = config.CFG.Pools[config.CFG.PoolIndex].Url
		poolIndex = config.CFG.PoolIndex

		newMiner := config.MinerInfo{}
		newMiner.IP = minerIp
		newMiner.PoolUrl = poolUrl

		config.CFG.Miners = append(config.CFG.Miners, newMiner)

	} else {

		for idx, pool := range config.CFG.Pools {
			if pool.Url == poolUrl {
				poolIndex = uint64(idx)
				break
			}
		}
	}

	return poolUrl, poolIndex

}

// Create new upstream for incomming connection from miner
func CreateNewUpstream(conn *stratumserver.Connection) error {

	venuslog.Warn("Trying to create new upstream")

	newId := LatestUpstream + 1
	client := &stratumclient.Client{}

	venuslog.Warn("Trying to Upstream ID", newId)

	minerIp := strings.Split(conn.Conn.RemoteAddr().String(), ":")[0]

	venuslog.Warn("Trying to Upstream ID", minerIp)

	poolUrl, poolIndex := findPoolUrl(minerIp)

	conn.PoolId = poolIndex

	err := client.Connect(poolUrl, newId)

	if err != nil {
		venuslog.Warn("Error while sending connecting to pool")
		conn.Close()
		return err
	}

	UpstreamsMut.Lock()

	Upstreams[newId] = &Upstream{
		ID:     newId,
		client: client,
		server: conn,
	}
	Upstreams[newId].Jobs = make(map[uint64]*Job, 100)
	conn.Upstream = newId

	UpstreamsMut.Unlock()

	makeReport()

	go handleDownstream(newId)

	venuslog.Warn("New upstream id ", newId, config.CFG.Pools[conn.PoolId].Url)

	return nil
}

// Sending mining.subscribe msg of stratum to mining pool
func SendSubscribe(conn *stratumserver.Connection, data []byte) {

	if conn.Upstream == 0 {
		err := CreateNewUpstream(conn)

		if err != nil {
			venuslog.Warn("Error while sending subscribe to pool")
			return
		}
	}

	venuslog.Warn("Trying to send")

	err := Upstreams[conn.Upstream].client.SendData(data)

	if err != nil {
		venuslog.Warn("Error while sending subscribe to pool")
	}
}

// Sending mining.configure msg of stratum to mining pool
func SendConfigure(conn *stratumserver.Connection, data []byte) {

	if conn.Upstream == 0 {
		err := CreateNewUpstream(conn)

		if err != nil {
			venuslog.Warn("Error while sending configure to pool")
			return
		}
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

		switch req.Method {
		case "mining.notify":

			venuslog.Warn("Stratum proxy received job from pool :")

			notifymsg := template.NotifyMsg{}
			errJson := rpc.ReadJSON(&notifymsg, msg)

			if errJson != nil {
				venuslog.Warn("ReadJSON failed in proxy from pool:", errJson)
				return
			}

			jobId, errUintJob := strconv.ParseUint(notifymsg.Params[0].(string), 16, 64)

			if errUintJob != nil {
				venuslog.Warn("ReadJSON failed in proxy from pool:", notifymsg.Params[0].(string), errUintJob)
				return
			}

			difficulty, errUintDiff := strconv.ParseUint(notifymsg.Params[6].(string), 16, 64)

			venuslog.Warn("ReadJSON Job ID:", difficulty)

			if errUintDiff != nil {
				venuslog.Warn("ReadJSON failed in proxy from pool:", notifymsg.Params[6].(string), errUintDiff)
				return
			}

			Upstreams[upstreamId].Jobs[jobId] = &Job{}

			Upstreams[upstreamId].Jobs[jobId].Difficulty = difficulty
			Upstreams[upstreamId].Jobs[jobId].Status = 1

			Upstreams[upstreamId].Shares.Accepted++
			Upstreams[upstreamId].server.Shares.Accepted++

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

		UpstreamsMut.Lock()

		upstream.Close()

		UpstreamsMut.Unlock()

		venuslog.Warn("Deleted miner", remoteAddr)

	}
	return nil
}

// Closing all connections
func closeAllUpstream() {

	UpstreamsMut.Lock()

	for _, us := range Upstreams {
		us.Close()
	}

	UpstreamsMut.Unlock()

	venuslog.Warn("Closed All Upstream")
}

// Closing connections from miner
func closeAllUpstreamFromMiner(minerIpStr string) {

	UpstreamsMut.Lock()

	for _, us := range Upstreams {

		minerIpOfUpstream := strings.Split(us.server.Conn.RemoteAddr().String(), ":")[0]

		if minerIpOfUpstream != minerIpStr {
			continue
		}

		us.Close()
	}

	UpstreamsMut.Unlock()

	venuslog.Warn("Closed All Upstream From Miner", minerIpStr)
}
