// SPDX-License-Identifier: Apache-2.0

package predict

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The ONNX predictor is a declared integration point, not an implementation.
// These assertions keep it honest: it identifies itself as unreleased and
// returns nothing, so it can never be mistaken for a working model.
func TestONNXStubIsInertButIdentifiable(t *testing.T) {
	var p *ONNXPredictor
	require.Equal(t, "onnx", p.Name())
	require.Equal(t, "unreleased", p.Version())
	require.Nil(t, p.Predict(Features{PeakRainfallMM: 100, MeanMaxTempC: 40}))

	// It satisfies the Predictor interface, so wiring it later is a swap.
	var _ Predictor = p
}
