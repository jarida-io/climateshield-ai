// SPDX-License-Identifier: Apache-2.0

package predict

// ONNXPredictor will run the trained gradient-boosted model exported to ONNX.
// TODO(Q1): implement once the model exists; until then the constructor
// reports ErrNotImplemented and Select falls back to the rules engine only
// when no model path is configured (a configured-but-missing model is a hard
// startup error, never a silent fallback).
type ONNXPredictor struct{}

// NewONNXPredictor returns ErrNotImplemented: no model has been trained yet.
func NewONNXPredictor(modelPath string) (*ONNXPredictor, error) {
	_ = modelPath
	return nil, ErrNotImplemented
}

// Name implements Predictor.
func (*ONNXPredictor) Name() string { return "onnx" }

// Version implements Predictor.
func (*ONNXPredictor) Version() string { return "unreleased" }

// Predict implements Predictor. Unreachable until NewONNXPredictor succeeds.
func (*ONNXPredictor) Predict(Features) []Prediction { return nil }
