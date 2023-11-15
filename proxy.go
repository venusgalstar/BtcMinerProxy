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
	"bufio"
	"io"
	"time"
)

var srv = stratumserver.Server{}

func StartProxy() {
	go func() {
		for {
			newConn := <-srv.NewConnections
			go HandleConnection(newConn)
		}
	}()

	for i, v := range config.CFG.Bind {
		if i != len(config.CFG.Bind)-1 {
			go srv.Start(v.Port, v.Host, v.Tls, v.PoolId)
		} else {
			srv.Start(v.Port, v.Host, v.Tls, v.PoolId)
		}
	}
}

// Handling Upstreaming Message and Data
func HandleConnection(conn *stratumserver.Connection) {
	for {
		req := template.StratumMsg{}
		conn.Conn.SetReadDeadline(time.Now().Add(config.READ_TIMEOUT_SECONDS * time.Second))
		reader := bufio.NewReaderSize(conn.Conn, config.MAX_REQUEST_SIZE)
		data, isPrefix, errR := reader.ReadLine()

		if errR != nil || isPrefix {
			if errR == io.EOF {
				continue
			}
			venuslog.Warn("Read Data failed in proxy from miner:", errR)
			Kick(conn.Id)
			return
		}

		err := rpc.ReadJSON(&req, data)

		if err != nil {
			venuslog.Warn("ReadJSON failed in proxy from miner:", err)
			Kick(conn.Id)
			return
		}

		switch req.Method {
		case "mining.subscribe":
			venuslog.Warn("Stratum proxy received subscribing msg from miner :", conn.Conn.RemoteAddr())

			str := string(data[:])
			venuslog.Warn("data:", str)

			subscribeReq := template.SubscribeMsg{}
			err := rpc.ReadJSON(&subscribeReq, data)
			if err != nil {
				venuslog.Warn("ReadJSON failed in proxy from miner:", err)
				Kick(conn.Id)
				return
			}

			SendSubscribe(conn, data)

		case "mining.authorize":
			venuslog.Warn("Stratum proxy received authorize from miner :", conn.Conn.RemoteAddr())
			str := string(data[:])
			venuslog.Warn("data:", str)

			Upstreams[conn.Upstream].client.SendData(data)
		default:
			venuslog.Warn("Stratum proxy received data from miner :", conn.Conn.RemoteAddr())
			Upstreams[conn.Upstream].client.SendData(data)
		}
	}
}

// Note: srv.ConnsMut must be locked before calling this
func Kick(id uint64) {
	for i, v := range srv.Connections {
		if v.Id == id {
			// Close the connection
			v.Conn.Close()

			if Upstreams[v.Upstream] != nil {
				// remove client from upstream
				UpstreamsMut.Lock()
				// If upstream is empty, close it
				Upstreams[v.Upstream].Close()

				UpstreamsMut.Unlock()
			}

			// remove client from server connections
			if len(srv.Connections) > 1 {
				srv.Connections = append(srv.Connections[:i], srv.Connections[i+1:]...)
			} else {
				srv.Connections = make([]*stratumserver.Connection, 0, 100)
			}
		}
	}
}
