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
	checkIndex := func(ae *netlink.AttributeEncoder) {
		ae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, 1)
	}

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, fn := range fns {
				t.Run(fn.name, func(t *testing.T) {
					c := testClient(t, func(_ genetlink.Message, _ netlink.Message) ([]genetlink.Message, error) {
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

			c := testClient(t, func(greq genetlink.Message, req netlink.Message) ([]genetlink.Message, error) {
				// Verify the parameters of the requests which are unique to
				// the LinkInfo call.
				if diff := cmp.Diff(netlink.Request|netlink.Dump, req.Header.Flags); diff != "" {
					t.Fatalf("unexpected netlink flags (-want +got):\n%s", diff)
				}

				if diff := cmp.Diff(unix.ETHTOOL_MSG_LINKINFO_GET, int(greq.Header.Command)); diff != "" {
					t.Fatalf("unexpected ethtool command (-want +got):\n%s", diff)
				}

				// The request must have a link info header with only flags,
				// no requests for an individual interface.
				b := encode(t, func(ae *netlink.AttributeEncoder) {
					ae.Nested(unix.ETHTOOL_A_LINKINFO_HEADER, func(nae *netlink.AttributeEncoder) error {
						nae.Uint32(unix.ETHTOOL_A_HEADER_FLAGS, unix.ETHTOOL_FLAG_COMPACT_BITSETS)
						return nil
					})
				})

				if diff := cmp.Diff(b, greq.Data); diff != "" {
					t.Fatalf("unexpected request header bytes (-want +got):\n%s", diff)
				}

				// All clear, return the expected canned data.
				return msgs, nil
			})
			defer c.Close()

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
			name: "by index",
			r:    Request{Index: 1},
			check: func(ae *netlink.AttributeEncoder) {
				ae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, 1)
			},
			li: &LinkInfo{
				Index: 1,
				Name:  "eth0",
				Port:  TwistedPair,
			},
		},
		{
			name: "by name",
			r:    Request{Name: "eth1"},
			check: func(ae *netlink.AttributeEncoder) {
				ae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, "eth1")
			},
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
			check: func(ae *netlink.AttributeEncoder) {
				ae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, 2)
				ae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, "eth1")
			},
			li: &LinkInfo{
				Index: 2,
				Name:  "eth1",
				Port:  DirectAttach,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, func(greq genetlink.Message, req netlink.Message) ([]genetlink.Message, error) {
				// Verify the parameters of the requests which are unique to
				// the LinkInfoByInterface calls.
				if diff := cmp.Diff(netlink.Request, req.Header.Flags); diff != "" {
					t.Fatalf("unexpected netlink flags (-want +got):\n%s", diff)
				}

				if diff := cmp.Diff(unix.ETHTOOL_MSG_LINKINFO_GET, int(greq.Header.Command)); diff != "" {
					t.Fatalf("unexpected ethtool command (-want +got):\n%s", diff)
				}

				b := encode(t, func(ae *netlink.AttributeEncoder) {
					ae.Nested(unix.ETHTOOL_A_LINKINFO_HEADER, func(nae *netlink.AttributeEncoder) error {
						// Apply additional attributes via the check function so
						// that we can call both the index and name methods
						// without duplicating much more logic.
						tt.check(nae)
						nae.Uint32(unix.ETHTOOL_A_HEADER_FLAGS, unix.ETHTOOL_FLAG_COMPACT_BITSETS)
						return nil
					})
				})

				if diff := cmp.Diff(b, greq.Data); diff != "" {
					t.Fatalf("unexpected request header bytes (-want +got):\n%s", diff)
				}

				return []genetlink.Message{encodeLinkInfo(t, *tt.li)}, nil
			})
			defer c.Close()

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

			c := testClient(t, func(greq genetlink.Message, req netlink.Message) ([]genetlink.Message, error) {
				// Verify the parameters of the requests which are unique to
				// the LinkMode call.
				if diff := cmp.Diff(netlink.Request|netlink.Dump, req.Header.Flags); diff != "" {
					t.Fatalf("unexpected netlink flags (-want +got):\n%s", diff)
				}

				if diff := cmp.Diff(unix.ETHTOOL_MSG_LINKMODES_GET, int(greq.Header.Command)); diff != "" {
					t.Fatalf("unexpected ethtool command (-want +got):\n%s", diff)
				}

				// The request must have a link mode header with only flags,
				// no requests for an individual interface.
				b := encode(t, func(ae *netlink.AttributeEncoder) {
					ae.Nested(unix.ETHTOOL_A_LINKMODES_HEADER, func(nae *netlink.AttributeEncoder) error {
						nae.Uint32(unix.ETHTOOL_A_HEADER_FLAGS, unix.ETHTOOL_FLAG_COMPACT_BITSETS)
						return nil
					})
				})

				if diff := cmp.Diff(b, greq.Data); diff != "" {
					t.Fatalf("unexpected request header bytes (-want +got):\n%s", diff)
				}

				// All clear, return the expected canned data.
				return msgs, nil
			})
			defer c.Close()

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
			name: "by index",
			r:    Request{Index: 1},
			check: func(ae *netlink.AttributeEncoder) {
				ae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, 1)
			},
			li: &LinkMode{
				Index:         1,
				Name:          "eth0",
				SpeedMegabits: 1000,
				Duplex:        Half,
			},
		},
		{
			name: "by name",
			r:    Request{Name: "eth1"},
			check: func(ae *netlink.AttributeEncoder) {
				ae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, "eth1")
			},
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
			check: func(ae *netlink.AttributeEncoder) {
				ae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, 2)
				ae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, "eth1")
			},
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
			c := testClient(t, func(greq genetlink.Message, req netlink.Message) ([]genetlink.Message, error) {
				// Verify the parameters of the requests which are unique to
				// the LinkModeByInterface calls.
				if diff := cmp.Diff(netlink.Request, req.Header.Flags); diff != "" {
					t.Fatalf("unexpected netlink flags (-want +got):\n%s", diff)
				}

				if diff := cmp.Diff(unix.ETHTOOL_MSG_LINKMODES_GET, int(greq.Header.Command)); diff != "" {
					t.Fatalf("unexpected ethtool command (-want +got):\n%s", diff)
				}

				b := encode(t, func(ae *netlink.AttributeEncoder) {
					ae.Nested(unix.ETHTOOL_A_LINKMODES_HEADER, func(nae *netlink.AttributeEncoder) error {
						// Apply additional attributes via the check function so
						// that we can call both the index and name methods
						// without duplicating much more logic.
						tt.check(nae)
						nae.Uint32(unix.ETHTOOL_A_HEADER_FLAGS, unix.ETHTOOL_FLAG_COMPACT_BITSETS)
						return nil
					})
				})

				if diff := cmp.Diff(b, greq.Data); diff != "" {
					t.Fatalf("unexpected request header bytes (-want +got):\n%s", diff)
				}

				return []genetlink.Message{encodeLinkMode(t, *tt.li)}, nil
			})
			defer c.Close()

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

const familyID = 20

func testClient(t *testing.T, fn genltest.Func) *Client {
	t.Helper()

	family := genetlink.Family{
		ID:      familyID,
		Version: unix.ETHTOOL_GENL_VERSION,
		Name:    unix.ETHTOOL_GENL_NAME,
	}

	conn := genltest.Dial(genltest.ServeFamily(family, fn))

	c, err := initClient(conn)
	if err != nil {
		t.Fatalf("failed to open client: %v", err)
	}

	return &Client{c: c}
}
