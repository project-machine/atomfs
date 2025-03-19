package squashfs

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsEmpytDir(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	v, e := isEmptyDir("/")
	assert.NoError(e)
	assert.False(v)

	v, e = isEmptyDir("/root")
	assert.Error(e)

	dname, err := os.MkdirTemp("", "squashfs_empty_test_dir")
	assert.NoError(err)
	defer os.RemoveAll(dname)

	v, e = isEmptyDir(dname)
	assert.NoError(e)
	assert.True(v)
}
