package sftpfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostKeyPolicy(t *testing.T) {
	_, err := Dialer{}.HostKeyCallback()
	assert.Error(t, err, "no pin and no explicit opt-out must refuse, never accept silently")

	cb, err := Dialer{AllowAnyHostKey: true}.HostKeyCallback()
	require.NoError(t, err)
	assert.NotNil(t, cb)

	_, err = Dialer{AuthorizedKey: "not a key"}.HostKeyCallback()
	assert.Error(t, err)
}

func TestHasExt(t *testing.T) {
	assert.True(t, hasExt("Weekly.XLSX", []string{".xlsx"}))
	assert.False(t, hasExt("weekly.csv", []string{".xlsx"}))
	assert.True(t, hasExt("anything", nil))
}
