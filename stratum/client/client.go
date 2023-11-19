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

// package stratumclient implements a Cryptonote Stratum mining protocol client
package stratumclient

import (
	"btcminerproxy/config"
	"btcminerproxy/mutex"
	"btcminerproxy/venuslog"
	"net"
	"time"
)

const ()

type SubmitWorkResult struct {
	Status string `json:"status"`
}

type Client struct {
	destination string
	upstreamId  uint64
	mutex       mutex.Mutex
	Conn        net.Conn
	alive       bool
}

func (cl *Client) IsAlive() bool {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	return cl.alive
}

// Connect client socket to mining pool
func (cl *Client) Connect(destination string, upstream uint64) (err error) {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()

	cl.destination = destination
	cl.Conn, err = net.DialTimeout("tcp", destination, time.Second*config.WRITE_TIMEOUT_SECONDS)
	cl.Conn.SetWriteDeadline(time.Now().Add(config.WRITE_TIMEOUT_SECONDS * time.Second))

	if err != nil {
		return err
	}
	cl.upstreamId = upstream
	cl.alive = true
	return nil
}

// Send subscribe through client socket to mining pool
func (cl *Client) SendSubscribe(destination string, data []byte, upstream uint64) (err error) {

	cl.mutex.Lock()
	defer cl.mutex.Unlock()

	data = append(data, '\n')
	if _, err = cl.Conn.Write(data); err != nil {
		venuslog.Warn(err)
		return err
	}

	venuslog.Warn("sent subscribe to pool")

	return nil
}

// Send data through client socket to mining pool
func (cl *Client) SendData(data []byte) (err error) {

	cl.Conn.SetWriteDeadline(time.Now().Add(config.WRITE_TIMEOUT_SECONDS * time.Second))
	if _, err = cl.Conn.Write(append(data, '\n')); err != nil {
		venuslog.Warn(err)
		return err
	}
	return nil
}

// Closing socket
func (cl *Client) Close() {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	if !cl.alive {
		return
	}
	cl.alive = false
	cl.Conn.Close()
}
