package lib

import (
	"bytes"
	"fmt"
	"sync"
)

type BoundStackMap[T any] struct {
	capacity int
	data     []string
	dataMap  map[string]T
	cc       int // current capacity
	m        sync.RWMutex
}

// NewBoundStackMap creates a new map with the limited capacity, where
// new records overwrite the oldest ones. The map is thread-safe
func NewBoundStackMap[T any](size int) *BoundStackMap[T] {
	return &BoundStackMap[T]{
		capacity: size,
		data:     make([]string, 0, size),
		dataMap:  make(map[string]T, size),
		m:        sync.RWMutex{},
	}
}

func (bs *BoundStackMap[T]) Push(key string, item T) {
	bs.m.Lock()
	defer bs.m.Unlock()

	if bs.cc == bs.capacity {
		delete(bs.dataMap, bs.data[0])
		bs.data = bs.data[1:]
	} else {
		bs.cc++
	}
	bs.data = append(bs.data, key)
	bs.dataMap[key] = item
}

func (bs *BoundStackMap[T]) Get(key string) (T, bool) {
	bs.m.RLock()
	defer bs.m.RUnlock()

	item, ok := bs.dataMap[key]
	return item, ok
}

func (bs *BoundStackMap[T]) At(index int) (T, bool) {
	bs.m.RLock()
	defer bs.m.RUnlock()

	// adjustment for negative index values to be counted from the end
	if index < 0 {
		index = bs.cc + index
	}
	// check if index is out of bounds
	if index < 0 || index > (bs.cc-1) {
		return *new(T), false
	}
	return bs.dataMap[bs.data[index]], true
}

func (bs *BoundStackMap[T]) Clear() {
	bs.m.Lock()
	defer bs.m.Unlock()

	bs.cc = 0
	bs.data = make([]string, 0, bs.capacity)
	bs.dataMap = make(map[string]T)
}

func (bs *BoundStackMap[T]) Count() int {
	bs.m.RLock()
	defer bs.m.RUnlock()

	return bs.cc
}

func (bs *BoundStackMap[T]) Capacity() int {
	bs.m.RLock()
	defer bs.m.RUnlock()

	return bs.capacity
}

func (bs *BoundStackMap[T]) Keys() []string {
	bs.m.RLock()
	defer bs.m.RUnlock()
	return bs.data[:]
}

func (bs *BoundStackMap[T]) String() string {
	bs.m.RLock()
	defer bs.m.RUnlock()

	b := new(bytes.Buffer)
	for index, key := range bs.data {
		fmt.Fprintf(b, "(%d) %s: %v\n", index, key, bs.dataMap[key])
	}
	return b.String()
}
