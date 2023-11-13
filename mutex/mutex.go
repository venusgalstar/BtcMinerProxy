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

package mutex

import (
	"sync"
)

type Mutex struct {
	m sync.RWMutex
}

/*func getCaller() string {
	_, file, line, _ := runtime.Caller(2)
	f := strings.Split(file, "/")
	out := f[len(f)-1] + ":" + strconv.FormatInt(int64(line), 10)
	return out
}

var x int = 0*/

func (m *Mutex) Lock() {
	/*venuslog.Debug("Lock", getCaller())
	x++*/
	m.m.Lock()
	//venuslog.Debug("Lock successful (", x, ")")
}
func (m *Mutex) Unlock() {
	//venuslog.Debug("Unlock", getCaller())
	m.m.Unlock()
	/*x--
	venuslog.Debug("Unlock successful (", x, "remaining)")*/
}
