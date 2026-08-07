// SPDX-License-Identifier: Apache-2.0

package predict

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/platform/logging"
)

func TestSelectDefaultsToRulesAndLogsIt(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New(&buf, "info")

	p, err := Select("", "", log)
	require.NoError(t, err)
	require.Equal(t, "rules", p.Name())
	require.Contains(t, buf.String(), `"predictor":"rules"`)
	require.Contains(t, buf.String(), RulesVersion)
}

func TestSelectClimatology(t *testing.T) {
	var buf bytes.Buffer
	p, err := Select("climatology", "", logging.New(&buf, "info"))
	require.NoError(t, err)
	require.Equal(t, "climatology", p.Name())
	require.Equal(t, ClimatologyVersion, p.Version())
	require.Contains(t, buf.String(), `"predictor":"climatology"`)
}

func TestSelectUnknownPredictor(t *testing.T) {
	_, err := Select("astrology", "", logging.New(io.Discard, "info"))
	require.ErrorContains(t, err, "unknown predictor")
}

func TestSelectWithModelPathFailsLoudly(t *testing.T) {
	// A configured model that can't load must be a startup error — silently
	// falling back to rules would misreport scoring provenance.
	_, err := Select("rules", "/models/outbreak.onnx", logging.New(io.Discard, "info"))
	require.ErrorIs(t, err, ErrNotImplemented)
}

func TestONNXConstructorNotImplemented(t *testing.T) {
	p, err := NewONNXPredictor("x.onnx")
	require.ErrorIs(t, err, ErrNotImplemented)
	require.Nil(t, p)
}
