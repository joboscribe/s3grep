package cli

import (
	"testing"

	"github.com/joboscribe/s3grep/tool"
	"github.com/stretchr/testify/assert"
)

var (
	testRegexString1      = "testReg1"
	testRegexString2      = "testReg2"
	testRegexString3      = "testReg3"
	testBucket            = "testBucket"
	testKey               = "testKey"
	testRegion            = "testRegion"
	testDirectory         = "/here"
	testNumWorkers        = "200"
	notEnoughArgs         = []string{"arg1", "arg2", "arg3"}
	unknownFlag           = []string{"-g", "arg1", "arg2", "arg3", "arg4"}
	flagMissingArg        = []string{"-k", testRegexString1, testBucket, testKey, testRegion}
	defaultArgs           = []string{testRegexString1, testBucket, testKey, testRegion}
	nonDefaultArgs        = []string{"-i", "-k", testDirectory, "-e", testRegexString2, "--regexp", testRegexString3, "-n", testNumWorkers, testRegexString1, testBucket, testKey, testRegion}
	expectedDefaultConfig = tool.Config{
		RegexStrings:    []string{testRegexString1},
		Bucket:          testBucket,
		Region:          testRegion,
		NumWorkers:      10,
		CaseInsensitive: false,
		KeepDir:         "",
	}
	expectedNonDefaultConfig = tool.Config{
		RegexStrings:    []string{testRegexString1, testRegexString2, testRegexString3},
		Bucket:          testBucket,
		Keys:            []string{testKey},
		Region:          testRegion,
		NumWorkers:      200,
		CaseInsensitive: true,
		KeepDir:         "/here",
	}
)

func TestParse(t *testing.T) {
	// test some reject cases first
	_, err := Parse(notEnoughArgs)
	assert.NotNil(t, err)

	_, err = Parse(flagMissingArg)
	assert.NotNil(t, err)

	// test default happy path
	defaultConfig, err := Parse(defaultArgs)
	assert.Nil(t, err)
	assert.Equal(t, expectedDefaultConfig.RegexStrings, defaultConfig.RegexStrings)
	assert.Equal(t, expectedDefaultConfig.Bucket, defaultConfig.Bucket)
	assert.Equal(t, expectedDefaultConfig.Region, defaultConfig.Region)
	assert.Equal(t, expectedDefaultConfig.NumWorkers, defaultConfig.NumWorkers)
	assert.Equal(t, expectedDefaultConfig.CaseInsensitive, defaultConfig.CaseInsensitive)
	assert.Equal(t, expectedDefaultConfig.KeepDir, defaultConfig.KeepDir)

	// test non-default happy path
	nonDefaultConfig, err := Parse(nonDefaultArgs)
	assert.Nil(t, err)
	assert.ElementsMatch(t, expectedNonDefaultConfig.RegexStrings, nonDefaultConfig.RegexStrings)
	assert.Equal(t, expectedNonDefaultConfig.Bucket, nonDefaultConfig.Bucket)
	assert.Equal(t, expectedNonDefaultConfig.Region, nonDefaultConfig.Region)
	assert.Equal(t, expectedNonDefaultConfig.NumWorkers, nonDefaultConfig.NumWorkers)
	assert.Equal(t, expectedNonDefaultConfig.CaseInsensitive, nonDefaultConfig.CaseInsensitive)
	assert.Equal(t, expectedNonDefaultConfig.KeepDir, nonDefaultConfig.KeepDir)

}
