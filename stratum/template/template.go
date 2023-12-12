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

package template

import (
	"btcminerproxy/config"
	"btcminerproxy/venuslog"
	"bytes"
	"io"
	"net"
)

// Stratum Protocol
type StratumMsg struct {
	ID     uint64 `json:"id"`
	Method string `json:"method"`
}

type NotifyMsg struct {
	ID     uint64        `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

type SubmitMsg struct {
	ID     uint64   `json:"id"`
	Method string   `json:"method"`
	Params []string `json:"params"`
}

type SubscribeMsg struct {
	ID     uint64 `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type AuthorizeMsg struct {
	ID     uint64   `json:"id"`
	Method string   `json:"method"`
	Params []string `json:"params"`
}

type authorizeParams struct {
	User string `json:"user"`
	Pass string `json:"pass"`
}

type StratumMsgResponse struct {
	ID     uint64 `json:"id"`
	Result any    `json:"result"`
	Error  string `json:"error"`
}

type StratumSeverMsg struct {
	ID     uint64 `json:"id"`
	Method string `json:"method"`
}

// Read one stratum msg from Socket, because protocol is tcp, we need buffering
func ReadLineFromSocket(conn net.Conn, buf []byte, bufLen int) (line []byte, lineLen int, readLen int, err error) {

	if bufLen > 0 {
		firstLine := bytes.IndexByte(buf, '\n')

		if firstLine > 0 {
			slice := buf[0:firstLine]
			return slice, firstLine, 0, nil
		}
	}

	readBuf := make([]byte, config.MAX_REQUEST_SIZE)
	readBytes, err := conn.Read(readBuf)

	if err != nil {
		if err != io.EOF {
			return nil, 0, 0, err
		}
	}

	if readBytes == 0 {
		return nil, 0, 0, nil
	}

	if bufLen+readBytes >= config.MAX_REQUEST_SIZE {
		venuslog.Warn("Over Loaded")
		return nil, 0, readBytes, nil
	}

	for idx := 0; idx < readBytes; idx++ {
		buf[bufLen+idx] = readBuf[idx]
	}

	firstLine := bytes.IndexByte(buf, '\n')

	if firstLine == -1 || firstLine == 0 {
		return nil, 0, readBytes, nil
	}

	slice := buf[0:firstLine]
	return slice, firstLine, readBytes, nil
}
