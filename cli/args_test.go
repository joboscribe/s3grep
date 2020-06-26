package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	notEnoughArgs = []string{"arg1", "arg2", "arg3"}
	unknownFlag   = []string{"-g", "arg1", "arg2", "arg3", "arg4"}
	validArgs     = []string{"testReg", "testBucket", "testKey", "testRegion"}
)

func TestParse(t *testing.T) {
	_, err := Parse(notEnoughArgs)
	assert.NotNil(t, err)

	_, err = Parse(validArgs)
	assert.Nil(t, err)
}
