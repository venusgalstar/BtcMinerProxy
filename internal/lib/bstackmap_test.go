package lib

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBstackMapCount(t *testing.T) {
	bsm := makeSampleBSM()
	require.Equal(t, 3, bsm.Count())
}

func TestBstackMapCapacity(t *testing.T) {
	bsm := makeSampleBSM()
	bsm.Push("fourth", 4)
	require.Equal(t, 3, bsm.Count())
	require.Equal(t, 3, bsm.Capacity())
}

func TestBstackMapOverwrite(t *testing.T) {
	bsm := makeSampleBSM()
	bsm.Push("fourth", 4)
	_, ok := bsm.Get("first")
	require.False(t, ok)
}

func TestBstackMapAt(t *testing.T) {
	bsm := makeSampleBSM()
	item, _ := bsm.At(0)
	require.Equal(t, 1, item)

	item, _ = bsm.At(-1)
	require.Equal(t, 3, item)
}

func TestBstackMapAtNegative(t *testing.T) {
	bsm := makeSampleBSM()
	item, _ := bsm.At(-1)
	require.Equal(t, 3, item)
}

func TestBstackMapAtOutOfBounds(t *testing.T) {
	bsm := makeSampleBSM()
	_, ok := bsm.At(bsm.Count())
	require.False(t, ok)
}

func TestBstackMapAtOutOfBoundsNegative(t *testing.T) {
	bsm := makeSampleBSM()
	_, ok := bsm.At(-bsm.Count() - 1)
	require.False(t, ok)
}

func TestBstackMapClear(t *testing.T) {
	bsm := makeSampleBSM()
	bsm.Clear()
	require.Equal(t, 0, bsm.Count())
	require.Equal(t, 3, bsm.Capacity())

	_, ok := bsm.Get("second")
	require.False(t, ok)

	_, ok = bsm.At(0)
	require.False(t, ok)
}

func makeSampleBSM() *BoundStackMap[int] {
	bsm := NewBoundStackMap[int](3)
	bsm.Push("first", 1)
	bsm.Push("second", 2)
	bsm.Push("third", 3)
	return bsm
}
