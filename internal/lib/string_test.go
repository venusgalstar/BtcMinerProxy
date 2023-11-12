package lib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeFilename(t *testing.T) {
	src := "2023-03-03T13:57:52+01:00-!!!@@@##$^&-stratum.slushpool.com:3333.log"
	exp := "2023-03-03T13-57-52+01-00-stratum.slushpool.com-3333.log"
	res := SanitizeFilename(src)
	assert.Equal(t, res, exp)
}
