// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/jarida-io/climateshield/internal/predict"
)

// The reference artifact is committed to the repository and its SHA-256 is
// published on /v1/model, so its formatting is part of its identity: a
// regenerated file that differs only in whitespace would read as a different
// artifact. This encoder therefore writes the committed layout exactly —
// every object key sorted, one space of indent per level, every float
// carrying a decimal point, one trailing newline.
//
// encode_test.go proves it byte for byte against the committed file: decode
// that file, re-encode it here, compare. Standard library marshalling cannot
// do this (it emits struct fields in declaration order and prints 0 for a
// zero float), which is why this exists.

// encodeClimatology renders c in the committed artifact's exact layout.
func encodeClimatology(c *predict.Climatology) []byte {
	w := &jsonWriter{}
	w.openObject()
	w.key("counties")
	w.openObject()
	for i, id := range sortedKeys(c.Counties) {
		w.comma(i)
		w.key(id)
		w.openObject()
		w.key("months")
		w.openObject()
		county := c.Counties[id]
		for j, m := range sortedKeys(county.Months) {
			w.comma(j)
			w.key(m)
			w.writeMonth(county.Months[m])
		}
		w.closeObject() // months
		w.closeObject() // county
	}
	w.closeObject() // counties

	w.comma(1)
	w.key("generated_by")
	w.writeString(c.GeneratedBy)
	w.comma(1)
	w.key("quantile_steps_pct")
	w.writeIntArray(c.QuantileStepsPct)
	w.comma(1)
	w.key("reference_period")
	w.writeString(c.ReferencePeriod)
	w.comma(1)
	w.key("schema_version")
	w.writeInt(c.SchemaVersion)
	w.comma(1)
	w.key("source")
	w.writeString(c.Source)
	w.comma(1)
	w.key("source_licence")
	w.writeString(c.SourceLicence)
	w.comma(1)
	w.key("window_days")
	w.writeInt(c.WindowDays)
	w.closeObject() // document

	w.buf.WriteByte('\n')
	return w.buf.Bytes()
}

func (w *jsonWriter) writeMonth(m predict.Month) {
	w.openObject()
	w.key("quantiles")
	w.openObject()
	for i, driver := range sortedKeys(m.Quantiles) {
		w.comma(i)
		w.key(driver)
		w.writeFloatArray(m.Quantiles[driver])
	}
	w.closeObject()
	w.comma(1)
	w.key("samples")
	w.writeInt(m.Samples)
	w.closeObject()
}

// jsonWriter emits the layout described above. depth counts open containers.
type jsonWriter struct {
	buf   bytes.Buffer
	depth int
}

func (w *jsonWriter) pad() { w.buf.WriteString(strings.Repeat(" ", w.depth)) }

// comma writes the separator before every element except the first.
func (w *jsonWriter) comma(index int) {
	if index > 0 {
		w.buf.WriteString(",\n")
		w.pad()
	}
}

func (w *jsonWriter) openObject() {
	w.buf.WriteString("{\n")
	w.depth++
	w.pad()
}

func (w *jsonWriter) closeObject() {
	w.buf.WriteByte('\n')
	w.depth--
	w.pad()
	w.buf.WriteByte('}')
}

func (w *jsonWriter) key(name string) {
	w.writeString(name)
	w.buf.WriteString(": ")
}

func (w *jsonWriter) writeString(s string) {
	// A JSON encoder with HTML escaping off matches Python's json.dumps for
	// the ASCII metadata this artifact carries.
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	w.buf.WriteString(strings.TrimRight(b.String(), "\n"))
}

func (w *jsonWriter) writeInt(v int) { w.buf.WriteString(strconv.Itoa(v)) }

func (w *jsonWriter) writeIntArray(vs []int) {
	w.openArray()
	for i, v := range vs {
		w.comma(i)
		w.writeInt(v)
	}
	w.closeArray()
}

func (w *jsonWriter) writeFloatArray(vs []float64) {
	w.openArray()
	for i, v := range vs {
		w.comma(i)
		w.buf.WriteString(formatFloat(v))
	}
	w.closeArray()
}

func (w *jsonWriter) openArray() {
	w.buf.WriteString("[\n")
	w.depth++
	w.pad()
}

func (w *jsonWriter) closeArray() {
	w.buf.WriteByte('\n')
	w.depth--
	w.pad()
	w.buf.WriteByte(']')
}

// formatFloat writes the shortest representation that round-trips, always
// with a decimal point so a whole number reads as the measurement it is.
func formatFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// sortedKeys returns a map's keys in ascending string order.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
