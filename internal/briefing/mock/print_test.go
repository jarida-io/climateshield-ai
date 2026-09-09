// SPDX-License-Identifier: Apache-2.0

package mock_test

import (
	"testing"

	"github.com/jarida-io/climateshield/internal/briefing/facts"
	"github.com/jarida-io/climateshield/internal/briefing/facts/factstest"
	"github.com/jarida-io/climateshield/internal/briefing/mock"
)

// TestPrintTemplates is a reading aid: `go test -run TestPrintTemplates -v`
// shows exactly what a county health officer is served when no language model
// ran. It asserts nothing the other tests do not already assert.
func TestPrintTemplates(t *testing.T) {
	for _, lang := range facts.Languages {
		body, err := mock.Template(factstest.Sample(), lang, mock.Notice{Kind: mock.NoticeNoModel})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("\n--- %s ---\n%s\n", lang, body)
	}
}
