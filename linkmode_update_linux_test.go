//go:build linux

package ethtool

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

func TestLinuxClientUpdateLinkModeAdvertise(t *testing.T) {
	for _, tt := range []struct {
		name  string
		bits  []int
		words []uint32
	}{
		{name: "empty", words: []uint32{}},
		{name: "one word", bits: []int{4, 31}, words: []uint32{0x80000010}},
		{name: "two words", bits: []int{4, 32, 63}, words: []uint32{0x10, 0x80000001}},
		{name: "three words", bits: []int{5, 33, 64}, words: []uint32{0x20, 2, 1}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			advertise := new(big.Int)
			for _, bit := range tt.bits {
				advertise.SetBit(advertise, bit, 1)
			}
			c := testClient(t, clientTest{
				HeaderFlags: netlink.Request | netlink.Acknowledge,
				Command:     unix.ETHTOOL_MSG_LINKMODES_SET,
				Attributes: func(ae *netlink.AttributeEncoder) {
					requestName(unix.ETHTOOL_A_LINKMODES_HEADER)(ae)
					ae.Nested(unix.ETHTOOL_A_LINKMODES_OURS, func(nae *netlink.AttributeEncoder) error {
						nae.Flag(unix.ETHTOOL_A_BITSET_NOMASK, true)
						nae.Uint32(unix.ETHTOOL_A_BITSET_SIZE, uint32(advertise.BitLen()))
						// Compact bitmaps send the least significant uint32 first,
						// with each word in native byte order.
						var b []byte
						for _, word := range tt.words {
							b = binary.NativeEndian.AppendUint32(b, word)
						}
						nae.Bytes(unix.ETHTOOL_A_BITSET_VALUE, b)
						return nil
					})
				},
				Messages: []genetlink.Message{{}},
			})
			defer c.Close()
			if err := c.UpdateLinkMode(Interface{Name: "eth1"}, &LinkModeUpdate{Advertise: advertise}); err != nil {
				t.Fatalf("failed to update link mode: %v", err)
			}
		})
	}
}
