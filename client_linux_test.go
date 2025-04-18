//go:build linux
// +build linux

package ethtool

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/josharian/native"
	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/genetlink/genltest"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

func TestLinuxClientErrors(t *testing.T) {
	tests := []struct {
		name  string
		ifi   Interface
		errno int
		err   error
	}{
		{
			name: "empty request",
			err:  errBadRequest,
		},
		{
			name:  "ENODEV",
			ifi:   Interface{Index: 1},
			errno: int(unix.ENODEV),
			err:   os.ErrNotExist,
		},
		{
			name:  "EOPNOTSUPP",
			ifi:   Interface{Index: 1},
			errno: int(unix.EOPNOTSUPP),
			err:   os.ErrNotExist,
		},
	}

	fns := []struct {
		name string
		call func(c *Client, ifi Interface) error
	}{
		{
			name: "link info",
			call: func(c *Client, ifi Interface) error {
				_, err := c.LinkInfo(ifi)
				return err
			},
		},
		{
			name: "link mode",
			call: func(c *Client, ifi Interface) error {
				_, err := c.LinkMode(ifi)
				return err
			},
		},
		{
			name: "link state",
			call: func(c *Client, ifi Interface) error {
				_, err := c.LinkState(ifi)
				return err
			},
		},
		{
			name: "wake on lan",
			call: func(c *Client, ifi Interface) error {
				_, err := c.WakeOnLAN(ifi)
				return err
			},
		},
		{
			name: "private flags",
			call: func(c *Client, ifi Interface) error {
				_, err := c.PrivateFlags(ifi)
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

					err := fn.call(c, tt.ifi)
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
					Interface: Interface{
						Index: 1,
						Name:  "eth0",
					},
					Port: TwistedPair,
				},
				{
					Interface: Interface{
						Index: 2,
						Name:  "eth1",
					},
					Port: DirectAttach,
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
				HeaderFlags: netlink.Request | netlink.Dump,
				Command:     unix.ETHTOOL_MSG_LINKINFO_GET,
				Attributes:  requestHeader(unix.ETHTOOL_A_LINKINFO_HEADER),

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
		ifi   Interface
		attrs func(ae *netlink.AttributeEncoder)
		li    *LinkInfo
	}{
		{
			name:  "by index",
			ifi:   Interface{Index: 1},
			attrs: requestIndex(unix.ETHTOOL_A_LINKINFO_HEADER, true),
			li: &LinkInfo{
				Interface: Interface{
					Index: 1,
					Name:  "eth0",
				},
				Port: TwistedPair,
			},
		},
		{
			name:  "by name",
			ifi:   Interface{Name: "eth1"},
			attrs: requestName(unix.ETHTOOL_A_LINKINFO_HEADER),
			li: &LinkInfo{
				Interface: Interface{
					Index: 2,
					Name:  "eth1",
				},
				Port: DirectAttach,
			},
		},
		{
			name: "both",
			ifi: Interface{
				Index: 2,
				Name:  "eth1",
			},
			attrs: requestBoth(unix.ETHTOOL_A_LINKINFO_HEADER, true),
			li: &LinkInfo{
				Interface: Interface{
					Index: 2,
					Name:  "eth1",
				},
				Port: DirectAttach,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, clientTest{
				HeaderFlags: netlink.Request,
				Command:     unix.ETHTOOL_MSG_LINKINFO_GET,
				Attributes:  tt.attrs,

				Messages: []genetlink.Message{encodeLinkInfo(t, *tt.li)},
			})

			li, err := c.LinkInfo(tt.ifi)
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
	// See https://github.com/mdlayher/ethtool/issues/12.
	//
	// It's likely that the bitset code is incorrect on big endian machines.
	skipBigEndian(t)

	tests := []struct {
		name string
		lms  []*LinkMode
	}{
		{
			name: "OK",
			lms: []*LinkMode{
				{
					Interface: Interface{
						Index: 1,
						Name:  "eth0",
					},
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
					Duplex:  Half,
					Autoneg: AutonegOff,
				},
				{
					Interface: Interface{
						Index: 2,
						Name:  "eth1",
					},
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
					Duplex:  Full,
					Autoneg: AutonegOn,
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
				HeaderFlags: netlink.Request | netlink.Dump,
				Command:     unix.ETHTOOL_MSG_LINKMODES_GET,
				Attributes:  requestHeader(unix.ETHTOOL_A_LINKMODES_HEADER),

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
		ifi   Interface
		attrs func(ae *netlink.AttributeEncoder)
		li    *LinkMode
	}{
		{
			name:  "by index",
			ifi:   Interface{Index: 1},
			attrs: requestIndex(unix.ETHTOOL_A_LINKMODES_HEADER, true),
			li: &LinkMode{
				Interface: Interface{
					Index: 1,
					Name:  "eth0",
				},
				SpeedMegabits: 1000,
				Duplex:        Half,
			},
		},
		{
			name:  "by name",
			ifi:   Interface{Name: "eth1"},
			attrs: requestName(unix.ETHTOOL_A_LINKMODES_HEADER),
			li: &LinkMode{
				Interface: Interface{
					Index: 2,
					Name:  "eth1",
				},
				SpeedMegabits: 10000,
				Duplex:        Full,
			},
		},
		{
			name: "both",
			ifi: Interface{
				Index: 2,
				Name:  "eth1",
			},
			attrs: requestBoth(unix.ETHTOOL_A_LINKMODES_HEADER, true),
			li: &LinkMode{
				Interface: Interface{
					Index: 2,
					Name:  "eth1",
				},
				SpeedMegabits: 10000,
				Duplex:        Full,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, clientTest{
				HeaderFlags: netlink.Request,
				Command:     unix.ETHTOOL_MSG_LINKMODES_GET,
				Attributes:  tt.attrs,

				Messages: []genetlink.Message{encodeLinkMode(t, *tt.li)},
			})

			li, err := c.LinkMode(tt.ifi)
			if err != nil {
				t.Fatalf("failed to get link mode: %v", err)
			}

			if diff := cmp.Diff(tt.li, li); diff != "" {
				t.Fatalf("unexpected link mode (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLinuxClientLinkStates(t *testing.T) {
	tests := []struct {
		name string
		lss  []*LinkState
	}{
		{
			name: "OK",
			lss: []*LinkState{
				{
					Interface: Interface{
						Index: 1,
						Name:  "eth0",
					},
				},
				{
					Interface: Interface{
						Index: 2,
						Name:  "eth1",
					},
					Link: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate the expected response messages using the wanted list
			// of LinkInfo structures.
			var msgs []genetlink.Message
			for _, ls := range tt.lss {
				msgs = append(msgs, encodeLinkState(t, *ls))
			}

			c := testClient(t, clientTest{
				HeaderFlags: netlink.Request | netlink.Dump,
				Command:     unix.ETHTOOL_MSG_LINKSTATE_GET,
				Attributes:  requestHeader(unix.ETHTOOL_A_LINKSTATE_HEADER),

				Messages: msgs,
			})

			lss, err := c.LinkStates()
			if err != nil {
				t.Fatalf("failed to get link states: %v", err)
			}

			if diff := cmp.Diff(tt.lss, lss); diff != "" {
				t.Fatalf("unexpected link states (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLinuxClientLinkState(t *testing.T) {
	tests := []struct {
		name  string
		ifi   Interface
		attrs func(ae *netlink.AttributeEncoder)
		ls    *LinkState
	}{
		{
			name:  "by index",
			ifi:   Interface{Index: 1},
			attrs: requestIndex(unix.ETHTOOL_A_LINKSTATE_HEADER, true),
			ls: &LinkState{
				Interface: Interface{
					Index: 1,
					Name:  "eth0",
				},
			},
		},
		{
			name:  "by name",
			ifi:   Interface{Name: "eth1"},
			attrs: requestName(unix.ETHTOOL_A_LINKSTATE_HEADER),
			ls: &LinkState{
				Interface: Interface{
					Index: 2,
					Name:  "eth1",
				},
				Link: true,
			},
		},
		{
			name: "both",
			ifi: Interface{
				Index: 2,
				Name:  "eth1",
			},
			attrs: requestBoth(unix.ETHTOOL_A_LINKSTATE_HEADER, true),
			ls: &LinkState{
				Interface: Interface{
					Index: 2,
					Name:  "eth1",
				},
				Link: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, clientTest{
				HeaderFlags: netlink.Request,
				Command:     unix.ETHTOOL_MSG_LINKSTATE_GET,
				Attributes:  tt.attrs,

				Messages: []genetlink.Message{encodeLinkState(t, *tt.ls)},
			})

			ls, err := c.LinkState(tt.ifi)
			if err != nil {
				t.Fatalf("failed to get link state: %v", err)
			}

			if diff := cmp.Diff(tt.ls, ls); diff != "" {
				t.Fatalf("unexpected link state (-want +got):\n%s", diff)
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
					Interface: Interface{
						Index: 1,
						Name:  "eth0",
					},
					Modes: Magic | MagicSecure,
				},
				{
					Interface: Interface{
						Index: 2,
						Name:  "eth1",
					},
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
				HeaderFlags: netlink.Request | netlink.Dump,
				Command:     unix.ETHTOOL_MSG_WOL_GET,
				Attributes:  requestHeader(unix.ETHTOOL_A_WOL_HEADER),

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
		ifi        Interface
		attrs      func(ae *netlink.AttributeEncoder)
		wol        WakeOnLAN
		nlErr, err error
	}{
		{
			name:  "EPERM",
			ifi:   Interface{Index: 1},
			attrs: requestIndex(unix.ETHTOOL_A_WOL_HEADER, true),
			nlErr: genltest.Error(int(unix.EPERM)),
			err:   os.ErrPermission,
		},
		{
			name:  "ok by index",
			ifi:   Interface{Index: 1},
			attrs: requestIndex(unix.ETHTOOL_A_WOL_HEADER, true),
			wol: WakeOnLAN{
				Interface: Interface{
					Index: 1,
					Name:  "eth0",
				},
			},
		},
		{
			name:  "ok by name",
			ifi:   Interface{Name: "eth1"},
			attrs: requestName(unix.ETHTOOL_A_WOL_HEADER),
			wol: WakeOnLAN{
				Interface: Interface{
					Index: 2,
					Name:  "eth1",
				},
			},
		},
		{
			name: "ok both",
			ifi: Interface{
				Index: 2,
				Name:  "eth1",
			},
			attrs: requestBoth(unix.ETHTOOL_A_WOL_HEADER, true),
			wol: WakeOnLAN{
				Interface: Interface{
					Index: 2,
					Name:  "eth1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, clientTest{
				HeaderFlags: netlink.Request,
				Command:     unix.ETHTOOL_MSG_WOL_GET,
				Attributes:  tt.attrs,

				Messages: []genetlink.Message{encodeWOL(t, tt.wol)},
				Error:    tt.nlErr,
			})

			wol, err := c.WakeOnLAN(tt.ifi)
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

func TestLinuxClientSetWakeOnLAN(t *testing.T) {
	wol := WakeOnLAN{
		Interface: Interface{
			Index: 2,
			Name:  "eth1",
		},
		Modes: Unicast | Magic,
	}

	tests := []struct {
		name       string
		wol        WakeOnLAN
		attrs      func(ae *netlink.AttributeEncoder)
		nlErr, err error
	}{
		{
			name:  "EPERM",
			wol:   WakeOnLAN{Interface: Interface{Index: 1}},
			attrs: requestIndex(unix.ETHTOOL_A_WOL_HEADER, false),
			nlErr: genltest.Error(int(unix.EPERM)),
			err:   os.ErrPermission,
		},
		{
			name: "ok",
			attrs: func(ae *netlink.AttributeEncoder) {
				// Apply the request header followed immediately by the rest of
				// the encoded WOL attributes.
				requestBoth(unix.ETHTOOL_A_WOL_HEADER, false)(ae)
				wol.encode(ae)
			},
			wol: wol,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, clientTest{
				HeaderFlags: netlink.Request | netlink.Acknowledge,
				Command:     unix.ETHTOOL_MSG_WOL_SET,
				Attributes:  tt.attrs,

				Messages: []genetlink.Message{encodeWOL(t, tt.wol)},
				Error:    tt.nlErr,
			})

			err := c.SetWakeOnLAN(tt.wol)
			if diff := cmp.Diff(tt.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Fatalf("unexpected error(-want +got):\n%s", diff)
			}
		})
	}
}

func requestHeader(typ uint16) func(*netlink.AttributeEncoder) {
	return func(ae *netlink.AttributeEncoder) {
		ae.Nested(typ, func(nae *netlink.AttributeEncoder) error {
			headerFlags(nae)
			return nil
		})
	}
}

func requestIndex(typ uint16, compactBitsets bool) func(*netlink.AttributeEncoder) {
	return func(ae *netlink.AttributeEncoder) {
		ae.Nested(typ, func(nae *netlink.AttributeEncoder) error {
			nae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, 1)
			if compactBitsets {
				headerFlags(nae)
			}
			return nil
		})
	}
}

func requestName(typ uint16) func(*netlink.AttributeEncoder) {
	return func(ae *netlink.AttributeEncoder) {
		ae.Nested(typ, func(nae *netlink.AttributeEncoder) error {
			nae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, "eth1")
			headerFlags(nae)
			return nil
		})
	}
}

func requestBoth(typ uint16, compactBitsets bool) func(*netlink.AttributeEncoder) {
	return func(ae *netlink.AttributeEncoder) {
		ae.Nested(typ, func(nae *netlink.AttributeEncoder) error {
			nae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, 2)
			nae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, "eth1")
			if compactBitsets {
				headerFlags(nae)
			}
			return nil
		})
	}
}

func headerFlags(ae *netlink.AttributeEncoder) {
	ae.Uint32(unix.ETHTOOL_A_HEADER_FLAGS, unix.ETHTOOL_FLAG_COMPACT_BITSETS)
}

func encodeLinkInfo(t *testing.T, li LinkInfo) genetlink.Message {
	t.Helper()

	return genetlink.Message{
		Data: encode(t, func(ae *netlink.AttributeEncoder) {
			ae.Nested(unix.ETHTOOL_A_LINKINFO_HEADER, func(nae *netlink.AttributeEncoder) error {
				nae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, uint32(li.Interface.Index))
				nae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, li.Interface.Name)
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
				nae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, uint32(lm.Interface.Index))
				nae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, lm.Interface.Name)
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
			ae.Uint8(unix.ETHTOOL_A_LINKMODES_AUTONEG, uint8(lm.Autoneg))
		}),
	}
}

func encodeLinkState(t *testing.T, ls LinkState) genetlink.Message {
	t.Helper()

	return genetlink.Message{
		Data: encode(t, func(ae *netlink.AttributeEncoder) {
			ae.Nested(unix.ETHTOOL_A_LINKSTATE_HEADER, func(nae *netlink.AttributeEncoder) error {
				nae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, uint32(ls.Interface.Index))
				nae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, ls.Interface.Name)
				return nil
			})

			// uint8 boolean conversion.
			var link uint8
			if ls.Link {
				link = 1
			}

			ae.Uint8(unix.ETHTOOL_A_LINKSTATE_LINK, link)
		}),
	}
}

func encodeWOL(t *testing.T, wol WakeOnLAN) genetlink.Message {
	t.Helper()

	return genetlink.Message{
		Data: encode(t, func(ae *netlink.AttributeEncoder) {
			ae.Nested(unix.ETHTOOL_A_WOL_HEADER, func(nae *netlink.AttributeEncoder) error {
				nae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, uint32(wol.Interface.Index))
				nae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, wol.Interface.Name)
				return nil
			})

			wol.encode(ae)
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
	HeaderFlags       netlink.HeaderFlags
	Command           uint8
	Attributes        func(*netlink.AttributeEncoder)
	EncodedAttributes []byte

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
		if ct.Error != nil {
			// Assume success tests will verify the request and return the error
			// early to avoid creating excessive test fixtures.
			return nil, ct.Error
		}

		// Verify the base netlink/genetlink headers.
		if diff := cmp.Diff(ct.HeaderFlags, req.Header.Flags); diff != "" {
			t.Fatalf("unexpected netlink flags (-want +got):\n%s", diff)
		}

		if diff := cmp.Diff(ct.Command, greq.Header.Command); diff != "" {
			t.Fatalf("unexpected ethtool command (-want +got):\n%s", diff)
		}

		// Verify the caller's request data.
		encoded := ct.EncodedAttributes
		if encoded == nil {
			encoded = encode(t, ct.Attributes)
		}
		if diff := cmp.Diff(encoded, greq.Data); diff != "" {
			t.Fatalf("unexpected request header bytes (-want +got):\n%s", diff)
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

func TestFEC(t *testing.T) {
	skipBigEndian(t)

	// captured from wireshark on the nlmon0 interface when running:
	//
	// sudo ethtool --set-fec enp7s0 encoding rs
	//
	// corresponding strace:
	//
	// sendto(3<socket:[12533277]>, [{nlmsg_len=68, nlmsg_type=ethtool, nlmsg_flags=NLM_F_REQUEST|NLM_F_ACK, nlmsg_seq=2, nlmsg_pid=0},
	// "\x1e\x01\x00\x00\x10\x00\x01\x80"
	// "\x0b\x00\x02\x00\x65\x6e\x70\x37\x73\x30\x00\x00\x18\x00\x02\x80\x04\x00\x01\x00\x10\x00\x03\x80\x0c\x00\x01\x80\x07\x00\x02\x00\x52\x53\x00\x00\x05\x00\x03\x00\x00\x00\x00\x00"], 68, 0, {sa_family=AF_NETLINK, nl_pid=0, nl_groups=00000000}, 12) = 68

	c := testClient(t, clientTest{
		HeaderFlags:       netlink.Request | netlink.Acknowledge,
		Command:           unix.ETHTOOL_MSG_FEC_SET,
		EncodedAttributes: []byte("\x10\x00\x01\x80\x0b\x00\x02\x00\x65\x6e\x70\x37\x73\x30\x00\x00\x18\x00\x02\x80\x04\x00\x01\x00\x10\x00\x03\x80\x0c\x00\x01\x80\x07\x00\x02\x00\x52\x53\x00\x00\x05\x00\x03\x00\x00\x00\x00\x00"),
		Messages: []genetlink.Message{{
			Data: nil,
		}},
		Error: nil,
	})

	err := c.SetFEC(FEC{
		Interface: Interface{Name: "enp7s0"},
		Modes:     unix.ETHTOOL_FEC_RS,
	})
	if err != nil {
		t.Fatalf("failed to set FEC: %v", err)
	}
}

func TestPrivateFlags(t *testing.T) {
	// Reference value captured from ethtool --show-priv-flags eth0
	c := testClient(t, clientTest{
		HeaderFlags:       netlink.Request,
		Command:           unix.ETHTOOL_MSG_PRIVFLAGS_GET,
		EncodedAttributes: []byte("\x10\x00\x01\x80\x09\x00\x02\x00\x65\x74\x68\x30\x00\x00\x00\x00"),
		Messages: []genetlink.Message{{
			Data: []byte("\x18\x00\x01\x80\x08\x00\x01\x00\x02\x00\x00\x00\x09\x00\x02\x00\x65\x74\x68\x30\x00\x00\x00\x00\xb4\x01\x02\x80\x08\x00\x02\x00\x0d\x00\x00\x00\xa8\x01\x03\x80\x14\x00\x01\x80\x08\x00\x01\x00\x00\x00\x00\x00\x08\x00\x02\x00\x4d\x46\x50\x00\x24\x00\x01\x80\x08\x00\x01\x00\x01\x00\x00\x00\x18\x00\x02\x00\x74\x6f\x74\x61\x6c\x2d\x70\x6f\x72\x74\x2d\x73\x68\x75\x74\x64\x6f\x77\x6e\x00\x1c\x00\x01\x80\x08\x00\x01\x00\x02\x00\x00\x00\x10\x00\x02\x00\x4c\x69\x6e\x6b\x50\x6f\x6c\x6c\x69\x6e\x67\x00\x28\x00\x01\x80\x08\x00\x01\x00\x03\x00\x00\x00\x16\x00\x02\x00\x66\x6c\x6f\x77\x2d\x64\x69\x72\x65\x63\x74\x6f\x72\x2d\x61\x74\x72\x00\x00\x00\x04\x00\x03\x00\x1c\x00\x01\x80\x08\x00\x01\x00\x04\x00\x00\x00\x0e\x00\x02\x00\x76\x65\x62\x2d\x73\x74\x61\x74\x73\x00\x00\x00\x20\x00\x01\x80\x08\x00\x01\x00\x05\x00\x00\x00\x14\x00\x02\x00\x68\x77\x2d\x61\x74\x72\x2d\x65\x76\x69\x63\x74\x69\x6f\x6e\x00\x24\x00\x01\x80\x08\x00\x01\x00\x06\x00\x00\x00\x17\x00\x02\x00\x6c\x69\x6e\x6b\x2d\x64\x6f\x77\x6e\x2d\x6f\x6e\x2d\x63\x6c\x6f\x73\x65\x00\x00\x1c\x00\x01\x80\x08\x00\x01\x00\x07\x00\x00\x00\x0e\x00\x02\x00\x6c\x65\x67\x61\x63\x79\x2d\x72\x78\x00\x00\x00\x28\x00\x01\x80\x08\x00\x01\x00\x08\x00\x00\x00\x1b\x00\x02\x00\x64\x69\x73\x61\x62\x6c\x65\x2d\x73\x6f\x75\x72\x63\x65\x2d\x70\x72\x75\x6e\x69\x6e\x67\x00\x00\x20\x00\x01\x80\x08\x00\x01\x00\x09\x00\x00\x00\x14\x00\x02\x00\x64\x69\x73\x61\x62\x6c\x65\x2d\x66\x77\x2d\x6c\x6c\x64\x70\x00\x1c\x00\x01\x80\x08\x00\x01\x00\x0a\x00\x00\x00\x0b\x00\x02\x00\x72\x73\x2d\x66\x65\x63\x00\x00\x04\x00\x03\x00\x20\x00\x01\x80\x08\x00\x01\x00\x0b\x00\x00\x00\x0f\x00\x02\x00\x62\x61\x73\x65\x2d\x72\x2d\x66\x65\x63\x00\x00\x04\x00\x03\x00\x28\x00\x01\x80\x08\x00\x01\x00\x0c\x00\x00\x00\x1c\x00\x02\x00\x76\x66\x2d\x74\x72\x75\x65\x2d\x70\x72\x6f\x6d\x69\x73\x63\x2d\x73\x75\x70\x70\x6f\x72\x74\x00"),
		}},
		Error: nil,
	})
	f, err := c.PrivateFlags(Interface{Name: "eth0"})
	if err != nil {
		t.Fatalf("failed to get private flags: %v", err)
	}
	if len(f.Flags) != 13 {
		t.Errorf("expected 13 flags, got %d", len(f.Flags))
	}
	if _, ok := f.Flags["disable-fw-lldp"]; !ok {
		t.Errorf("expected flag disable-fw-lldp to be present, but it is not")
	}
	if !f.Flags["rs-fec"] {
		t.Error("expected rs-fec flag to be active")
	}
}

func TestSetPrivateFlags(t *testing.T) {
	// Reference value captured from ethtool --set-priv-flags eth0 disable-fw-lldp on
	c := testClient(t, clientTest{
		HeaderFlags:       netlink.Request | netlink.Acknowledge,
		Command:           unix.ETHTOOL_MSG_PRIVFLAGS_SET,
		EncodedAttributes: []byte("\x10\x00\x01\x80\x09\x00\x02\x00\x65\x74\x68\x30\x00\x00\x00\x00\x24\x00\x02\x80\x20\x00\x03\x80\x1c\x00\x01\x80\x14\x00\x02\x00\x64\x69\x73\x61\x62\x6c\x65\x2d\x66\x77\x2d\x6c\x6c\x64\x70\x00\x04\x00\x03\x00"),
		Messages: []genetlink.Message{{
			Data: nil,
		}},
		Error: nil,
	})
	err := c.SetPrivateFlags(PrivateFlags{
		Interface: Interface{Name: "eth0"},
		Flags: map[string]bool{
			"disable-fw-lldp": true,
		},
	})
	if err != nil {
		t.Fatalf("failed to set private flags: %v", err)
	}
}

func skipBigEndian(t *testing.T) {
	t.Helper()

	if binary.ByteOrder(native.Endian) == binary.BigEndian {
		t.Skip("skipping, this test requires a little endian machine")
	}
}
