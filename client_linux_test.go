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

func TestLinuxClientEmptyResponse(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T, c *Client)
		msgs []genetlink.Message
	}{
		{
			name: "link info",
			fn: func(t *testing.T, c *Client) {
				lis, err := c.LinkInfos()
				if err != nil {
					t.Fatalf("failed to get link info: %v", err)
				}

				if diff := cmp.Diff(0, len(lis)); diff != "" {
					t.Fatalf("unexpected number of link info structures (-want +got):\n%s", diff)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testClient(t, func(_ genetlink.Message, _ netlink.Message) ([]genetlink.Message, error) {
				return tt.msgs, nil
			})
			defer c.Close()

			tt.fn(t, c)
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

				// The request must only have a link info header with no nested
				// attributes since we're querying for all links.
				b := encode(t, func(ae *netlink.AttributeEncoder) {
					ae.Nested(unix.ETHTOOL_A_LINKINFO_HEADER, func(nae *netlink.AttributeEncoder) error {
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
	checkIndex := func(ae *netlink.AttributeEncoder) {
		ae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, 1)
	}

	tests := []struct {
		name  string
		r     Request
		check func(ae *netlink.AttributeEncoder)
		li    *LinkInfo
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
		{
			name:  "OK by index",
			r:     Request{Index: 1},
			check: checkIndex,
			li: &LinkInfo{
				Index: 1,
				Name:  "eth0",
				Port:  TwistedPair,
			},
		},
		{
			name: "OK by name",
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
			name: "OK both",
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
						return nil
					})
				})

				if diff := cmp.Diff(b, greq.Data); diff != "" {
					t.Fatalf("unexpected request header bytes (-want +got):\n%s", diff)
				}

				// Either return a netlink error number or canned data messages.
				if tt.errno != 0 {
					return nil, genltest.Error(tt.errno)
				}

				return []genetlink.Message{encodeLinkInfo(t, *tt.li)}, nil
			})
			defer c.Close()

			li, err := c.LinkInfo(tt.r)
			if tt.err == nil && err != nil {
				t.Fatalf("failed to get link info: %v", err)
			}
			if tt.err != nil && err == nil {
				t.Fatal("expected an error, but none occurred")
			}

			if diff := cmp.Diff(tt.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Fatalf("unexpected error (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(tt.li, li); diff != "" {
				t.Fatalf("unexpected link info (-want +got):\n%s", diff)
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
