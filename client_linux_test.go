//+build linux

package ethtool

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/genetlink/genltest"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

func TestLinuxClientErrors(t *testing.T) {
	tests := []struct {
		name  string
		r     Request
		check func(ae *netlink.AttributeEncoder)
		errno int
		err   error
	}{
		{
			name: "empty request",
			err:  errBadRequest,
		},
		{
			name:  "ENODEV",
			r:     Request{Index: 1},
			check: checkIndex,
			errno: int(unix.ENODEV),
			err:   os.ErrNotExist,
		},
		{
			name:  "EOPNOTSUPP",
			r:     Request{Index: 1},
			check: checkIndex,
			errno: int(unix.EOPNOTSUPP),
			err:   os.ErrNotExist,
		},
	}

	fns := []struct {
		name string
		call func(c *Client, r Request) error
	}{
		{
			name: "link info",
			call: func(c *Client, r Request) error {
				_, err := c.LinkInfo(r)
				return err
			},
		},
		{
			name: "link mode",
			call: func(c *Client, r Request) error {
				_, err := c.LinkMode(r)
				return err
			},
		},
		{
			name: "wake on lan",
			call: func(c *Client, r Request) error {
				_, err := c.WakeOnLAN(r)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, fn := range fns {
				t.Run(fn.name, func(t *testing.T) {
					c := baseClient(t, func(_ genetlink.Message, _ netlink.Message) ([]genetlink.Message, error) {
						return nil, genltest.Error(tt.errno)
					})
					defer c.Close()

					err := fn.call(c, tt.r)
					if diff := cmp.Diff(tt.err, err, cmpopts.EquateErrors()); diff != "" {
						t.Fatalf("unexpected error (-want +got):\n%s", diff)
					}
				})
			}
		})
	}
}

func TestLinuxClientLinkInfos(t *testing.T) {
	tests := []struct {
		name string
		lis  []*LinkInfo
	}{
		{
			name: "OK",
			lis: []*LinkInfo{
				{
					Index: 1,
					Name:  "eth0",
					Port:  TwistedPair,
				},
				{
					Index: 2,
					Name:  "eth1",
					Port:  DirectAttach,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate the expected response messages using the wanted list
			// of LinkInfo structures.
			var msgs []genetlink.Message
			for _, li := range tt.lis {
				msgs = append(msgs, encodeLinkInfo(t, *li))
			}

			c := testClient(t, clientTest{
				HeaderFlags:   netlink.Request | netlink.Dump,
				Command:       unix.ETHTOOL_MSG_LINKINFO_GET,
				RequestHeader: unix.ETHTOOL_A_LINKINFO_HEADER,

				Messages: msgs,
			})

			lis, err := c.LinkInfos()
			if err != nil {
				t.Fatalf("failed to get link info: %v", err)
			}

			if diff := cmp.Diff(tt.lis, lis); diff != "" {
				t.Fatalf("unexpected link info (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLinuxClientLinkInfo(t *testing.T) {
	tests := []struct {
		name  string
		r     Request
		check func(ae *netlink.AttributeEncoder)
		li    *LinkInfo
	}{
		{
			name:  "by index",
			r:     Request{Index: 1},
			check: checkIndex,
			li: &LinkInfo{
				Index: 1,
				Name:  "eth0",
				Port:  TwistedPair,
			},
		},
		{
			name:  "by name",
			r:     Request{Name: "eth1"},
			check: checkName,
			li: &LinkInfo{
				Index: 2,
				Name:  "eth1",
				Port:  DirectAttach,
			},
		},
		{
			name: "both",
			r: Request{
				Index: 2,
				Name:  "eth1",
			},
			check: checkBoth,
			li: &LinkInfo{
				Index: 2,
				Name:  "eth1",
				Port:  DirectAttach,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, clientTest{
				HeaderFlags:       netlink.Request,
				Command:           unix.ETHTOOL_MSG_LINKINFO_GET,
				RequestHeader:     unix.ETHTOOL_A_LINKINFO_HEADER,
				RequestAttributes: tt.check,

				Messages: []genetlink.Message{encodeLinkInfo(t, *tt.li)},
			})

			li, err := c.LinkInfo(tt.r)
			if err != nil {
				t.Fatalf("failed to get link info: %v", err)
			}

			if diff := cmp.Diff(tt.li, li); diff != "" {
				t.Fatalf("unexpected link info (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLinuxClientLinkModes(t *testing.T) {
	tests := []struct {
		name string
		lms  []*LinkMode
	}{
		{
			name: "OK",
			lms: []*LinkMode{
				{
					Index:         1,
					Name:          "eth0",
					SpeedMegabits: 1000,
					Ours: []AdvertisedLinkMode{
						{
							Index: unix.ETHTOOL_LINK_MODE_1000baseT_Half_BIT,
							Name:  "1000baseT/Half",
						},
						{
							Index: unix.ETHTOOL_LINK_MODE_1000baseT_Full_BIT,
							Name:  "1000baseT/Full",
						},
					},
					Duplex: Half,
				},
				{
					Index:         2,
					Name:          "eth1",
					SpeedMegabits: 10000,
					Ours: []AdvertisedLinkMode{
						{
							Index: unix.ETHTOOL_LINK_MODE_FIBRE_BIT,
							Name:  "FIBRE",
						},
						{
							Index: unix.ETHTOOL_LINK_MODE_10000baseT_Full_BIT,
							Name:  "10000baseT/Full",
						},
					},
					Duplex: Full,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate the expected response messages using the wanted list
			// of LinkMode structures.
			var msgs []genetlink.Message
			for _, lm := range tt.lms {
				msgs = append(msgs, encodeLinkMode(t, *lm))
			}

			c := testClient(t, clientTest{
				HeaderFlags:   netlink.Request | netlink.Dump,
				Command:       unix.ETHTOOL_MSG_LINKMODES_GET,
				RequestHeader: unix.ETHTOOL_A_LINKMODES_HEADER,

				Messages: msgs,
			})

			lms, err := c.LinkModes()
			if err != nil {
				t.Fatalf("failed to get link mode: %v", err)
			}

			if diff := cmp.Diff(tt.lms, lms); diff != "" {
				t.Fatalf("unexpected link mode (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLinuxClientLinkMode(t *testing.T) {
	tests := []struct {
		name  string
		r     Request
		check func(ae *netlink.AttributeEncoder)
		li    *LinkMode
	}{
		{
			name:  "by index",
			r:     Request{Index: 1},
			check: checkIndex,
			li: &LinkMode{
				Index:         1,
				Name:          "eth0",
				SpeedMegabits: 1000,
				Duplex:        Half,
			},
		},
		{
			name:  "by name",
			r:     Request{Name: "eth1"},
			check: checkName,
			li: &LinkMode{
				Index:         2,
				Name:          "eth1",
				SpeedMegabits: 10000,
				Duplex:        Full,
			},
		},
		{
			name: "both",
			r: Request{
				Index: 2,
				Name:  "eth1",
			},
			check: checkBoth,
			li: &LinkMode{
				Index:         2,
				Name:          "eth1",
				SpeedMegabits: 10000,
				Duplex:        Full,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, clientTest{
				HeaderFlags:       netlink.Request,
				Command:           unix.ETHTOOL_MSG_LINKMODES_GET,
				RequestHeader:     unix.ETHTOOL_A_LINKMODES_HEADER,
				RequestAttributes: tt.check,

				Messages: []genetlink.Message{encodeLinkMode(t, *tt.li)},
			})

			li, err := c.LinkMode(tt.r)
			if err != nil {
				t.Fatalf("failed to get link mode: %v", err)
			}

			if diff := cmp.Diff(tt.li, li); diff != "" {
				t.Fatalf("unexpected link mode (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLinuxClientWakeOnLANs(t *testing.T) {
	tests := []struct {
		name string
		wols []*WakeOnLAN
	}{
		{
			name: "OK",
			wols: []*WakeOnLAN{
				{
					Index: 1,
					Name:  "eth0",
					Modes: Magic | MagicSecure,
				},
				{
					Index: 2,
					Name:  "eth1",
					Modes: Magic,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate the expected response messages using the wanted list
			// of WakeOnLAN structures.
			var msgs []genetlink.Message
			for _, wol := range tt.wols {
				msgs = append(msgs, encodeWOL(t, *wol))
			}

			c := testClient(t, clientTest{
				HeaderFlags:   netlink.Request | netlink.Dump,
				Command:       unix.ETHTOOL_MSG_WOL_GET,
				RequestHeader: unix.ETHTOOL_A_WOL_HEADER,

				Messages: msgs,
			})

			wols, err := c.WakeOnLANs()
			if err != nil {
				t.Fatalf("failed to get link mode: %v", err)
			}

			if diff := cmp.Diff(tt.wols, wols); diff != "" {
				t.Fatalf("unexpected link mode (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLinuxClientWakeOnLAN(t *testing.T) {
	tests := []struct {
		name       string
		r          Request
		check      func(ae *netlink.AttributeEncoder)
		wol        WakeOnLAN
		nlErr, err error
	}{
		{
			name:  "EPERM",
			r:     Request{Index: 1},
			check: checkIndex,
			nlErr: genltest.Error(int(unix.EPERM)),
			err:   os.ErrPermission,
		},
		{
			name:  "ok by index",
			r:     Request{Index: 1},
			check: checkIndex,
			wol: WakeOnLAN{
				Index: 1,
				Name:  "eth0",
			},
		},
		{
			name:  "ok by name",
			r:     Request{Name: "eth1"},
			check: checkName,
			wol: WakeOnLAN{
				Index: 2,
				Name:  "eth1",
			},
		},
		{
			name: "ok both",
			r: Request{
				Index: 2,
				Name:  "eth1",
			},
			check: checkBoth,
			wol: WakeOnLAN{
				Index: 2,
				Name:  "eth1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, clientTest{
				HeaderFlags:       netlink.Request,
				Command:           unix.ETHTOOL_MSG_WOL_GET,
				RequestHeader:     unix.ETHTOOL_A_WOL_HEADER,
				RequestAttributes: tt.check,

				Messages: []genetlink.Message{encodeWOL(t, tt.wol)},
				Error:    tt.nlErr,
			})

			wol, err := c.WakeOnLAN(tt.r)
			if err != nil {
				if tt.err != nil {
					// This test expects an error, check it and skip the rest
					// of the comparisons.
					if diff := cmp.Diff(tt.err, err, cmpopts.EquateErrors()); diff != "" {
						t.Fatalf("unexpected error(-want +got):\n%s", diff)
					}

					return
				}

				t.Fatalf("failed to get Wake-on-LAN info: %v", err)
			}

			if diff := cmp.Diff(&tt.wol, wol); diff != "" {
				t.Fatalf("unexpected Wake-on-LAN (-want +got):\n%s", diff)
			}
		})
	}
}

func checkIndex(ae *netlink.AttributeEncoder) {
	ae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, 1)
}

func checkName(ae *netlink.AttributeEncoder) {
	ae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, "eth1")
}

func checkBoth(ae *netlink.AttributeEncoder) {
	ae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, 2)
	ae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, "eth1")
}

func encodeLinkInfo(t *testing.T, li LinkInfo) genetlink.Message {
	t.Helper()

	return genetlink.Message{
		Data: encode(t, func(ae *netlink.AttributeEncoder) {
			ae.Nested(unix.ETHTOOL_A_LINKINFO_HEADER, func(nae *netlink.AttributeEncoder) error {
				nae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, uint32(li.Index))
				nae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, li.Name)
				return nil
			})

			ae.Uint8(unix.ETHTOOL_A_LINKINFO_PORT, uint8(li.Port))
		}),
	}
}

func encodeLinkMode(t *testing.T, lm LinkMode) genetlink.Message {
	t.Helper()

	return genetlink.Message{
		Data: encode(t, func(ae *netlink.AttributeEncoder) {
			ae.Nested(unix.ETHTOOL_A_LINKMODES_HEADER, func(nae *netlink.AttributeEncoder) error {
				nae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, uint32(lm.Index))
				nae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, lm.Name)
				return nil
			})

			ae.Uint32(unix.ETHTOOL_A_LINKMODES_SPEED, uint32(lm.SpeedMegabits))

			packALMs := func(typ uint16, alms []AdvertisedLinkMode) {
				ae.Nested(typ, func(nae *netlink.AttributeEncoder) error {
					fn := packALMBitset(alms)
					nae.Uint32(unix.ETHTOOL_A_BITSET_SIZE, uint32(len(linkModes)))
					nae.Do(unix.ETHTOOL_A_BITSET_VALUE, fn)
					nae.Do(unix.ETHTOOL_A_BITSET_MASK, fn)
					return nil
				})
			}

			packALMs(unix.ETHTOOL_A_LINKMODES_OURS, lm.Ours)
			packALMs(unix.ETHTOOL_A_LINKMODES_PEER, lm.Peer)

			ae.Uint8(unix.ETHTOOL_A_LINKMODES_DUPLEX, uint8(lm.Duplex))
		}),
	}
}

func encodeWOL(t *testing.T, wol WakeOnLAN) genetlink.Message {
	t.Helper()

	return genetlink.Message{
		Data: encode(t, func(ae *netlink.AttributeEncoder) {
			ae.Nested(unix.ETHTOOL_A_WOL_HEADER, func(nae *netlink.AttributeEncoder) error {
				nae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, uint32(wol.Index))
				nae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, wol.Name)
				return nil
			})

			ae.Nested(unix.ETHTOOL_A_WOL_MODES, func(nae *netlink.AttributeEncoder) error {
				// TODO(mdlayher): ensure this stays in sync if new modes are added!
				nae.Uint32(unix.ETHTOOL_A_BITSET_SIZE, 8)

				// Note that we are cheating a bit here by directly passing a
				// uint32, but this is okay because there are less than 32 bits
				// for the WOL modes and therefore the bitset is just the native
				// endian representation of the modes bitmask.
				nae.Uint32(unix.ETHTOOL_A_BITSET_VALUE, uint32(wol.Modes))
				nae.Uint32(unix.ETHTOOL_A_BITSET_MASK, uint32(wol.Modes))
				return nil
			})
		}),
	}
}

func packALMBitset(alms []AdvertisedLinkMode) func() ([]byte, error) {
	return func() ([]byte, error) {
		// Calculate the number of words necessary for the bitset, then
		// multiply by 4 for bytes.
		b := make([]byte, ((len(linkModes)+31)/32)*4)

		for _, alm := range alms {
			byteIndex := alm.Index / 8
			bitIndex := alm.Index % 8
			b[byteIndex] |= 1 << bitIndex
		}

		return b, nil
	}
}

func encode(t *testing.T, fn func(ae *netlink.AttributeEncoder)) []byte {
	t.Helper()

	ae := netlink.NewAttributeEncoder()
	fn(ae)

	b, err := ae.Encode()
	if err != nil {
		t.Fatalf("failed to encode attributes: %v", err)
	}

	return b
}

// A clientTest is the input to testClient which defines expected request
// parameters and canned response messages and/or errors.
type clientTest struct {
	// Expected request parameters.
	HeaderFlags   netlink.HeaderFlags
	Command       uint8
	RequestHeader uint16
	// Optional: assert more attributes are part of a request.
	RequestAttributes func(*netlink.AttributeEncoder)

	// Response data. If Error is set, Messages is unused.
	Messages []genetlink.Message
	Error    error
}

// testClient produces a Client which handles request/responses by validating
// the parameters against those set in the clientTest structure. This is useful
// for high-level request/response testing.
func testClient(t *testing.T, ct clientTest) *Client {
	t.Helper()

	c := baseClient(t, func(greq genetlink.Message, req netlink.Message) ([]genetlink.Message, error) {
		// Verify the base netlink/genetlink headers.
		if diff := cmp.Diff(ct.HeaderFlags, req.Header.Flags); diff != "" {
			t.Fatalf("unexpected netlink flags (-want +got):\n%s", diff)
		}

		if diff := cmp.Diff(ct.Command, greq.Header.Command); diff != "" {
			t.Fatalf("unexpected ethtool command (-want +got):\n%s", diff)
		}

		// Encode a request header appropriate for this message so we can easily
		// compare against the caller's. Optionally apply more attributes for
		// checking if the caller is invoking a method by name/index/both.
		b := encode(t, func(ae *netlink.AttributeEncoder) {
			ae.Nested(ct.RequestHeader, func(nae *netlink.AttributeEncoder) error {
				if ct.RequestAttributes != nil {
					ct.RequestAttributes(nae)
				}

				// Always use compact bitsets.
				nae.Uint32(unix.ETHTOOL_A_HEADER_FLAGS, unix.ETHTOOL_FLAG_COMPACT_BITSETS)
				return nil
			})
		})

		if diff := cmp.Diff(b, greq.Data); diff != "" {
			t.Fatalf("unexpected request header bytes (-want +got):\n%s", diff)
		}

		// Either return an error or the messages.
		if ct.Error != nil {
			return nil, ct.Error
		}

		return ct.Messages, nil
	})
	t.Cleanup(func() {
		_ = c.Close()
	})

	return c
}

const familyID = 20

// baseClient produces a barebones Client with only the initial genetlink setup
// logic performed for low-level tests.
func baseClient(t *testing.T, fn genltest.Func) *Client {
	t.Helper()

	family := genetlink.Family{
		ID:      familyID,
		Version: unix.ETHTOOL_GENL_VERSION,
		Name:    unix.ETHTOOL_GENL_NAME,
		Groups: []genetlink.MulticastGroup{{
			ID:   1,
			Name: unix.ETHTOOL_MCGRP_MONITOR_NAME,
		}},
	}

	conn := genltest.Dial(genltest.ServeFamily(family, fn))

	c, err := initClient(conn)
	if err != nil {
		t.Fatalf("failed to open client: %v", err)
	}

	return &Client{c: c}
}
