// SPDX-License-Identifier: Apache-2.0

package predict

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// The digest reported to the public API must be the digest of the file a
// reviewer can hash on disk, or the provenance claim is worthless.
func TestClimatologyDigestMatchesTheFileOnDisk(t *testing.T) {
	raw, err := os.ReadFile(DefaultClimatologyFile)
	require.NoError(t, err)
	sum := sha256.Sum256(raw)

	digest, err := ClimatologyDigest()
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(sum[:]), digest)
	require.Len(t, digest, 64)

	_, err = climatologyDigestOf("climatologydata/does-not-exist.json")
	require.Error(t, err)
}

// The artifact names the tool that builds it, and that tool must exist at
// that path. This is the check that stops the file citing a generator nobody
// can run.
func TestArtifactNamesAGeneratorThatExists(t *testing.T) {
	c, err := LoadClimatology()
	require.NoError(t, err)
	require.Equal(t, "cmd/buildclimatology", c.GeneratedBy)

	info, err := os.Stat("../../" + c.GeneratedBy)
	require.NoError(t, err, "the artifact names a generator that must exist in this repository")
	require.True(t, info.IsDir())
}

// The counts published beside the model are measured from the artifact, not
// typed into a document.
func TestReferenceCountsAreMeasuredFromTheArtifact(t *testing.T) {
	c, err := LoadClimatology()
	require.NoError(t, err)

	require.Equal(t, 21, c.QuantileSteps())
	require.Equal(t, []string{"eldoret", "kisumu", "mombasa", "nairobi", "nakuru"}, c.CountyIDs())

	// Five counties x twelve months of 14-day windows over the reference
	// decade. The per-county total is the same for every county because they
	// all cover the same days.
	require.Equal(t, 18195, c.TotalSamples())
	perCounty := 0
	for _, m := range c.Counties["kisumu"].Months {
		perCounty += m.Samples
	}
	require.Equal(t, 3639, perCounty)
	require.Equal(t, perCounty*len(c.Counties), c.TotalSamples())
}

func TestLadderKeyCoversEveryPersistedDriver(t *testing.T) {
	for driver, want := range map[string]string{
		DriverPeakRainfall: driverPeakRain,
		DriverMeanMaxTemp:  driverMeanTmax,
		DriverMeanMinTemp:  driverMeanTmin,
	} {
		key, ok := LadderKey(driver)
		require.True(t, ok, driver)
		require.Equal(t, want, key)
	}
	_, ok := LadderKey("humidity_pct")
	require.False(t, ok)
}
