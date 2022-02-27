//go:build linux
// +build linux

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
			attrs: requestIndex(unix.ETHTOOL_A_LINKINFO_HEADER),
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
			attrs: requestBoth(unix.ETHTOOL_A_LINKINFO_HEADER),
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
					Duplex: Half,
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
			attrs: requestIndex(unix.ETHTOOL_A_LINKMODES_HEADER),
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
			attrs: requestBoth(unix.ETHTOOL_A_LINKMODES_HEADER),
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
			attrs: requestIndex(unix.ETHTOOL_A_LINKSTATE_HEADER),
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
			attrs: requestBoth(unix.ETHTOOL_A_LINKSTATE_HEADER),
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
			attrs: requestIndex(unix.ETHTOOL_A_WOL_HEADER),
			nlErr: genltest.Error(int(unix.EPERM)),
			err:   os.ErrPermission,
		},
		{
			name:  "ok by index",
			ifi:   Interface{Index: 1},
			attrs: requestIndex(unix.ETHTOOL_A_WOL_HEADER),
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
			attrs: requestBoth(unix.ETHTOOL_A_WOL_HEADER),
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
			attrs: requestIndex(unix.ETHTOOL_A_WOL_HEADER),
			nlErr: genltest.Error(int(unix.EPERM)),
			err:   os.ErrPermission,
		},
		{
			name: "ok",
			attrs: func(ae *netlink.AttributeEncoder) {
				// Apply the request header followed immediately by the rest of
				// the encoded WOL attributes.
				requestBoth(unix.ETHTOOL_A_WOL_HEADER)(ae)
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

func requestIndex(typ uint16) func(*netlink.AttributeEncoder) {
	return func(ae *netlink.AttributeEncoder) {
		ae.Nested(typ, func(nae *netlink.AttributeEncoder) error {
			nae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, 1)
			headerFlags(nae)
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

func requestBoth(typ uint16) func(*netlink.AttributeEncoder) {
	return func(ae *netlink.AttributeEncoder) {
		ae.Nested(typ, func(nae *netlink.AttributeEncoder) error {
			nae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, 2)
			nae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, "eth1")
			headerFlags(nae)
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

			ae.Nested(unix.ETHTOOL_A_LINKMODES_OURS, packALMs(lm.Ours))
			ae.Nested(unix.ETHTOOL_A_LINKMODES_PEER, packALMs(lm.Peer))

			ae.Uint8(unix.ETHTOOL_A_LINKMODES_DUPLEX, uint8(lm.Duplex))
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
	HeaderFlags netlink.HeaderFlags
	Command     uint8
	Attributes  func(*netlink.AttributeEncoder)

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
		if diff := cmp.Diff(encode(t, ct.Attributes), greq.Data); diff != "" {
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
