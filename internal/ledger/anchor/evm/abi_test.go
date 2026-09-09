// SPDX-License-Identifier: Apache-2.0

package evm_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jarida-io/climateshield/internal/ledger/anchor/evm"
)

// vectors mirrors testdata/golden/evm/vectors.json, generated with cast.
type vectors struct {
	AnchorSelector    string `json:"anchor_selector"`
	RootOfSelector    string `json:"rootOf_selector"`
	VersionsSelector  string `json:"versions_selector"`
	PublisherSelector string `json:"publisher_selector"`
	AnchoredTopic     string `json:"anchored_topic"`
	Day               string `json:"day"`
	DayBytes32        string `json:"day_bytes32"`
	Root              string `json:"root"`
	AnchorCalldata    string `json:"anchor_calldata"`
	RootOfCalldata    string `json:"rootOf_calldata"`
	VersionsCalldata  string `json:"versions_calldata"`
	PublisherCalldata string `json:"publisher_calldata"`
	VersionWord1      string `json:"version_word_1"`
}

func loadVectors(t *testing.T) vectors {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "golden", "evm", "vectors.json"))
	require.NoError(t, err)
	var v vectors
	require.NoError(t, json.Unmarshal(raw, &v))
	return v
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := evm.DecodeHex(s)
	require.NoError(t, err)
	return b
}

func vectorDay(t *testing.T, v vectors) (time.Time, [32]byte, [32]byte) {
	t.Helper()
	day, err := time.Parse(evm.DayLayout, v.Day)
	require.NoError(t, err)
	root, err := evm.Root32(mustHex(t, v.Root))
	require.NoError(t, err)
	return day, evm.DayBytes32(day), root
}

func TestSelectorsAndTopicMatchCast(t *testing.T) {
	v := loadVectors(t)
	require.Equal(t, mustHex(t, v.AnchorSelector), evm.SelectorAnchor)
	require.Equal(t, mustHex(t, v.RootOfSelector), evm.SelectorRootOf)
	require.Equal(t, mustHex(t, v.VersionsSelector), evm.SelectorVersions)
	require.Equal(t, mustHex(t, v.PublisherSelector), evm.SelectorPublisher)
	require.Equal(t, mustHex(t, v.AnchoredTopic), evm.AnchoredTopic)
	require.Len(t, evm.Selector("anything()"), 4)
}

func TestDayBytes32MatchesCastFormatBytes32String(t *testing.T) {
	v := loadVectors(t)
	day, day32, _ := vectorDay(t, v)
	require.Equal(t, mustHex(t, v.DayBytes32), day32[:])
	require.Equal(t, v.Day, evm.DayString(day32))

	// The day is taken in UTC regardless of the wall clock's zone.
	nairobi := time.FixedZone("EAT", 3*3600)
	require.Equal(t, day32, evm.DayBytes32(day.In(nairobi)))
}

func TestCalldataMatchesCast(t *testing.T) {
	v := loadVectors(t)
	_, day32, root := vectorDay(t, v)
	require.Equal(t, mustHex(t, v.AnchorCalldata), evm.PackAnchor(day32, root))
	require.Equal(t, mustHex(t, v.RootOfCalldata), evm.PackRootOf(day32))
	require.Equal(t, mustHex(t, v.VersionsCalldata), evm.PackVersions(day32))
	require.Equal(t, mustHex(t, v.PublisherCalldata), evm.PackPublisher())
}

func TestRoot32RequiresExactlyThirtyTwoNonZeroBytes(t *testing.T) {
	_, err := evm.Root32(make([]byte, 31))
	require.Error(t, err)
	_, err = evm.Root32(make([]byte, 33))
	require.Error(t, err)
	_, err = evm.Root32(make([]byte, 32))
	require.Error(t, err, "the contract rejects a zero root, so must we")
	good := make([]byte, 32)
	good[0] = 1
	r, err := evm.Root32(good)
	require.NoError(t, err)
	require.Equal(t, byte(1), r[0])
}

func TestUnpackers(t *testing.T) {
	v := loadVectors(t)

	_, err := evm.UnpackBytes32([]byte{1, 2, 3})
	require.Error(t, err)
	got, err := evm.UnpackBytes32(mustHex(t, v.Root))
	require.NoError(t, err)
	require.Equal(t, mustHex(t, v.Root), got[:])

	n, err := evm.UnpackUint64(mustHex(t, v.VersionWord1))
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	_, err = evm.UnpackUint64([]byte{1})
	require.Error(t, err)
	huge := make([]byte, 32)
	huge[0] = 1
	_, err = evm.UnpackUint64(huge)
	require.Error(t, err, "a value above 64 bits must not be silently truncated")

	word := make([]byte, 32)
	copy(word[12:], mustHex(t, "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266"))
	addr, err := evm.UnpackAddress(word)
	require.NoError(t, err)
	require.Equal(t, "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266", addr)
	word[0] = 1
	_, err = evm.UnpackAddress(word)
	require.Error(t, err)
	_, err = evm.UnpackAddress([]byte{1})
	require.Error(t, err)
}

func TestDecodeAnchored(t *testing.T) {
	v := loadVectors(t)
	_, day32, root := vectorDay(t, v)
	good := evm.Log{
		Topics: []string{v.AnchoredTopic, v.DayBytes32},
		Data:   evm.EncodeHex(append(append([]byte{}, root[:]...), mustHex(t, v.VersionWord1)...)),
	}
	ev, err := evm.DecodeAnchored(good)
	require.NoError(t, err)
	require.Equal(t, v.Day, ev.Day)
	require.Equal(t, root, ev.Root)
	require.EqualValues(t, 1, ev.Version)
	require.Equal(t, day32, evm.DayBytes32(mustParse(t, ev.Day)))

	for name, bad := range map[string]evm.Log{
		"one topic":     {Topics: []string{v.AnchoredTopic}, Data: good.Data},
		"wrong topic":   {Topics: []string{v.DayBytes32, v.DayBytes32}, Data: good.Data},
		"bad day hex":   {Topics: []string{v.AnchoredTopic, "0xzz"}, Data: good.Data},
		"short day":     {Topics: []string{v.AnchoredTopic, "0x01"}, Data: good.Data},
		"bad data hex":  {Topics: good.Topics, Data: "nothex"},
		"short data":    {Topics: good.Topics, Data: "0x01"},
		"huge version":  {Topics: good.Topics, Data: evm.EncodeHex(append(append([]byte{}, root[:]...), huge32()...))},
		"bad topic hex": {Topics: []string{"0xzz", v.DayBytes32}, Data: good.Data},
	} {
		_, err := evm.DecodeAnchored(bad)
		require.Error(t, err, name)
	}
}

func huge32() []byte {
	b := make([]byte, 32)
	b[0] = 0xff
	return b
}

func mustParse(t *testing.T, day string) time.Time {
	t.Helper()
	d, err := time.Parse(evm.DayLayout, day)
	require.NoError(t, err)
	return d
}

func TestLabelNeverCallsADevChainPublic(t *testing.T) {
	require.Equal(t, evm.DevChainLabel, evm.Label(31337))
	require.Equal(t, evm.DevChainLabel, evm.Label(1337))
	require.Contains(t, evm.DevChainLabel, "not a public network")
	require.True(t, evm.IsDevChain(31337))
	require.False(t, evm.IsDevChain(1))
	// Any other chain is named by number only; this code vouches for nothing.
	require.Equal(t, "chain id 1", evm.Label(1))
	require.Equal(t, "chain id 84532", evm.Label(84532))
}
