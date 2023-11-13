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

package rpc

import (
	"btcminerproxy/venuslog"
	"bufio"
	"encoding/json"
	"errors"
)

func ReadJSON(response any, reader *bufio.Reader) error {
	data, isPrefix, err := reader.ReadLine()

	if isPrefix {
		return errors.New("request")
	} else if err != nil {
		return err
	}
	err = json.Unmarshal(data, response)
	if err != nil {
		venuslog.Warn("json unmarshal failed:", err)
		return err
	}
	return nil
}
