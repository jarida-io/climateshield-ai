// SPDX-License-Identifier: Apache-2.0

package predict

import (
	"fmt"
)

// This file adds the reference climatology as an ANNOTATION on predictions
// that did not compute one — in practice, the fixed-threshold rules engine.
//
// It deliberately does not touch how a tier is decided. The published
// thresholds in rules.go are contractual, so RulesPredictor.Predict keeps
// returning exactly the levels it always has; the wrapper only fills in the
// Exceedance field and appends one sentence to the explanation. That is what
// makes it possible to show both scorers side by side against the same
// weather without the second one quietly changing an alert.

// ExceedanceRoleAnnotation is the plain-language statement published on
// /v1/model when the active predictor decides tiers from the published
// thresholds and the exceedance beside each score is only an annotation.
const ExceedanceRoleAnnotation = "The published thresholds decided every level below. " +
	"The exceedance beside a score is an annotation measured from the reference climatology " +
	"after the fact: it says how unusual that driver value is for that county and month, and " +
	"it did not move any tier."

// ExceedanceRoleDeciding is the statement published when the climatology
// predictor is active and the exceedance is what set the tier.
const ExceedanceRoleDeciding = "The exceedance decided every level below, at the declared " +
	"cut-points. It measures how unusual the driver value is for that county and month; it is " +
	"not a probability that an outbreak will occur."

// annotationTail says which end of the reference distribution the published
// rule treats as the hazard, so an annotation reports the same tail the rule
// itself reasons about. It restates the direction already encoded in
// rules.go (pneumonia is a cold-stress rule, the rest are upper-tail); it
// introduces no cutoff and changes no tier.
var annotationTail = map[Disease]bool{ // true = large values are the hazard
	Cholera:    true,
	Malaria:    true,
	Pneumonia:  false,
	Meningitis: true,
}

// annotationLadder maps a persisted driver label to the reference
// climatology's quantile ladder for the same quantity.
var annotationLadder = map[string]string{
	DriverPeakRainfall: driverPeakRain,
	DriverMeanMaxTemp:  driverMeanTmax,
	DriverMeanMinTemp:  driverMeanTmin,
}

// AnnotatedPredictor wraps any predictor and attaches the reference
// climatology's view of each driver value. Name and Version are the wrapped
// predictor's own, unchanged: a score row must record the scorer that decided
// it, and the annotation is not a scorer.
type AnnotatedPredictor struct {
	inner Predictor
	clim  *Climatology
}

// Annotate wraps p so that every prediction which carries no exceedance of
// its own gets one from clim. A nil climatology returns p untouched — an
// annotation that cannot be measured is left off rather than guessed.
func Annotate(p Predictor, clim *Climatology) Predictor {
	if clim == nil || p == nil {
		return p
	}
	return &AnnotatedPredictor{inner: p, clim: clim}
}

// Name implements Predictor: the wrapped predictor's name, because it is the
// one that decided the tier.
func (a *AnnotatedPredictor) Name() string { return a.inner.Name() }

// Version implements Predictor: the wrapped predictor's version.
func (a *AnnotatedPredictor) Version() string { return a.inner.Version() }

// Inner returns the wrapped predictor.
func (a *AnnotatedPredictor) Inner() Predictor { return a.inner }

// Predict implements Predictor. Levels, drivers and driver values come
// through byte-identical; only Exceedance and the tail of Explanation are
// added, and only where the wrapped predictor left Exceedance empty.
func (a *AnnotatedPredictor) Predict(f Features) []Prediction {
	out := a.inner.Predict(f)
	for i := range out {
		if out[i].Exceedance != nil {
			continue
		}
		exc, ok := a.exceedanceFor(out[i], f)
		if !ok {
			continue
		}
		e := exc
		out[i].Exceedance = &e
		out[i].Explanation = appendAnnotation(out[i].Explanation,
			annotationSentence(out[i], f, exc, a.clim.Samples(f.AreaID, f.Month)))
	}
	return out
}

// exceedanceFor measures the prediction's own driver value against the
// reference distribution for the same quantity, in the tail the published
// rule treats as hazardous.
func (a *AnnotatedPredictor) exceedanceFor(p Prediction, f Features) (float64, bool) {
	key, ok := annotationLadder[p.Driver]
	if !ok {
		return 0, false
	}
	upper, ok := annotationTail[p.Disease]
	if !ok {
		return 0, false
	}
	exc, err := a.clim.Exceedance(f.AreaID, f.Month, key, p.DriverValue, upper)
	if err != nil {
		return 0, false
	}
	return exc, true
}

// annotationSentence states what the reference record says about this driver
// value, and says in the same breath that it did not decide the tier.
func annotationSentence(p Prediction, f Features, exc float64, samples int) string {
	extreme, comparison := "highest", "this high"
	if !annotationTail[p.Disease] {
		extreme, comparison = "lowest", "this low"
	}
	if exc <= 0 {
		return fmt.Sprintf(
			"For reference, that is the %s value on record for %s in month %d "+
				"(%d reference windows); the published threshold, not this figure, set the level.",
			extreme, f.AreaID, f.Month, samples)
	}
	return fmt.Sprintf(
		"For reference, %.1f%% of reference windows for %s in month %d are at least %s "+
			"(%d reference windows); the published threshold, not this figure, set the level.",
		exc*100, f.AreaID, f.Month, comparison, samples)
}

func appendAnnotation(base, extra string) string {
	if base == "" {
		return extra
	}
	return base + ". " + extra
}
