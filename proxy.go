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
	"btcminerproxy/stratum/rpc"
	stratumserver "btcminerproxy/stratum/server"
	"btcminerproxy/stratum/template"
	"btcminerproxy/venuslog"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"
)

var srv = stratumserver.Server{}

// Main process of proxy, starting proxy depends config proxy
// in terms of port and monitoring incoming connection from miner
func StartProxy() {
	go func() {
		for {
			newConn := <-srv.NewConnections
			go HandleConnection(newConn)
		}
	}()

	for i, v := range config.CFG.Bind {
		if i != len(config.CFG.Bind)-1 {
			go srv.Start(v.Port, v.Host, v.Tls)
		} else {
			srv.Start(v.Port, v.Host, v.Tls)
		}
	}
}

func updatePoolRatedHash(conn *stratumserver.Connection, jobId uint64) {

	poolUrl := config.CFG.Pools[conn.PoolId].Url

	for idx, pool := range globalPoolInfo {

		if pool.PoolUrl != poolUrl {
			continue
		}

		globalPoolInfo[idx].RatedHash += Upstreams[conn.Upstream].Jobs[jobId].Difficulty
		Upstreams[conn.Upstream].Jobs[jobId].Status = 0

		delete(Upstreams[conn.Upstream].Jobs, jobId)
		return
	}

	t := time.Now()

	newPoolInfo := &PoolRatedHash{}
	newPoolInfo.PoolUrl = poolUrl
	newPoolInfo.RatedHash = 0
	newPoolInfo.Timestamp = t.String()

	globalPoolInfo = append(globalPoolInfo, *newPoolInfo)

}

// Handling Upstreaming Message and Data
func HandleConnection(conn *stratumserver.Connection) {

	buf := make([]byte, config.MAX_REQUEST_SIZE)
	bufLen := 0
	conn.Conn.SetReadDeadline(time.Now().Add(config.READ_TIMEOUT_SECONDS * time.Second))

	ipAddr := strings.Split(conn.Conn.RemoteAddr().String(), ":")

	result := checkBlackList(ipAddr[0])

	if result == true {
		venuslog.Info("This address is in blocklist", ipAddr[0])
		Kick(conn.Id)
		return
	}

	for {

		// Read data from socket and parsing stratum msg one by one
		req := template.StratumMsg{}
		msg, msgLen, readLen, err := template.ReadLineFromSocket(conn.Conn, buf, bufLen)

		if err != nil || msgLen == 0 {
			if err == io.EOF {
				continue
			}
			venuslog.Warn("Read Data failed in proxy from miner:", err)
			Kick(conn.Id)
			return
		}

		buf = buf[msgLen+1:]
		bufLen = bufLen + readLen - msgLen - 1

		errJson := rpc.ReadJSON(&req, msg)

		if errJson != nil {
			venuslog.Warn("ReadJSON failed in proxy from miner:", errJson)
			Kick(conn.Id)
			return
		}

		str := string(msg[:])
		venuslog.Warn("data:", str)

		// Recognizing message type and handling services
		switch req.Method {

		case "mining.subscribe":
			venuslog.Warn("Stratum proxy received subscribing msg from miner :", conn.Conn.RemoteAddr())
			SendSubscribe(conn, msg)

		case "mining.authorize":
			venuslog.Warn("Stratum proxy received authorize from miner :", conn.Conn.RemoteAddr())

			authorizemsg := template.AuthorizeMsg{}
			errJson := rpc.ReadJSON(&authorizemsg, msg)

			if errJson != nil {
				venuslog.Warn("ReadJSON failed in proxy from miner:", errJson)
				return
			}

			conn.WorkerID = authorizemsg.Params[0]

			authorizemsg.Params[0] = config.CFG.Pools[conn.PoolId].User
			authorizemsg.Params[1] = config.CFG.Pools[conn.PoolId].Pass

			newmsg, err := json.Marshal(authorizemsg)
			if err != nil {
				venuslog.Warn("ReadJSON failed in proxy replace auth:", err)
				return
			}

			SendData(conn, newmsg)

		case "mining.configure":
			venuslog.Warn("Stratum proxy received configure from miner :", conn.Conn.RemoteAddr())
			SendConfigure(conn, msg)

		case "mining.submits":
			submitmsg := template.SubmitMsg{}
			errJson := rpc.ReadJSON(&submitmsg, msg)

			if errJson != nil {
				venuslog.Warn("ReadJSON failed in proxy from miner:", errJson)
				return
			}

			jobId, errUintJob := strconv.ParseUint(submitmsg.Params[1], 10, 0)

			if errUintJob != nil {
				venuslog.Warn("ReadJSON failed in proxy from miner:", submitmsg.Params[1], errUintJob)
				return
			}

			updatePoolRatedHash(conn, jobId)

			conn.Submits.Accepted++
			Upstreams[conn.Upstream].Submits.Accepted++

		default:
			venuslog.Warn("Stratum proxy received data from miner :", conn.Conn.RemoteAddr())
			SendData(conn, msg)
		}

		copy(buf, buf[msgLen+1:])
	}
}

// Note: srv.ConnsMut must be locked before calling this
func Kick(id uint64) {
	for i, v := range srv.Connections {
		if v.Id == id {
			// Close the connection
			v.Conn.Close()

			UpstreamsMut.Lock()

			if Upstreams[v.Upstream] != nil {
				// remove client from upstream

				// If upstream is empty, close it
				Upstreams[v.Upstream].Close()
			}

			UpstreamsMut.Unlock()

			makeReport()

			// remove client from server connections
			if len(srv.Connections) > 1 {
				srv.Connections = append(srv.Connections[:i], srv.Connections[i+1:]...)
			} else {
				srv.Connections = make([]*stratumserver.Connection, 0, 100)
			}
		}
	}
}
