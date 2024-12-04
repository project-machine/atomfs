package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type uidmapTestcase struct {
	uidmap   string
	expected bool
}

var uidmapTests = []uidmapTestcase{
	{
		uidmap:   `         0          0 4294967295`,
		expected: true,
	},
	{
		uidmap: `         0          0 1000
2000 2000 1`,
		expected: false,
	},
	{
		uidmap:   `         0          0 1000`,
		expected: false,
	},
	{
		uidmap:   `         10          0 4294967295`,
		expected: false,
	},
	{
		uidmap:   `         0          10 4294967295`,
		expected: false,
	},
	{
		uidmap:   `         0          0 1`,
		expected: false,
	},
}

func TestAmHostRoot(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	for _, testcase := range uidmapTests {
		v := uidmapIsHost(testcase.uidmap)
		assert.Equal(v, testcase.expected)
	}
}
