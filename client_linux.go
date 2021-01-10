//+build linux

package ethtool

import (
	"errors"
	"fmt"
	"os"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

// errBadRequest indicates an invalid Request from the caller.
var errBadRequest = errors.New("ethtool: Request must have Index and/or Name set when calling Client methods")

// A client is the Linux implementation backing a Client.
type client struct {
	c      *genetlink.Conn
	family uint16
}

// newClient opens a generic netlink connection to the ethtool family.
func newClient() (*client, error) {
	conn, err := genetlink.Dial(nil)
	if err != nil {
		return nil, err
	}

	c, err := initClient(conn)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	return c, nil
}

// initClient is the internal client constructor used in some tests.
func initClient(c *genetlink.Conn) (*client, error) {
	f, err := c.GetFamily(unix.ETHTOOL_GENL_NAME)
	if err != nil {
		return nil, err
	}

	// TODO(mdlayher): look into what exactly the ethtool interface does with
	// extended acknowledgements and consider setting them there.

	return &client{
		c:      c,
		family: f.ID,
	}, nil
}

// Close closes the underlying generic netlink connection.
func (c *client) Close() error { return c.c.Close() }

// execute executes the specified command with additional header flags and input
// netlink request attributes. The netlink.Request header flag is automatically
// set.
func (c *client) execute(cmd uint8, flags netlink.HeaderFlags, ae *netlink.AttributeEncoder) ([]genetlink.Message, error) {
	b, err := ae.Encode()
	if err != nil {
		return nil, err
	}

	return c.c.Execute(
		genetlink.Message{
			Header: genetlink.Header{
				Command: cmd,
				Version: unix.ETHTOOL_GENL_VERSION,
			},
			Data: b,
		},
		// Always pass the genetlink family ID and request flag.
		c.family,
		netlink.Request|flags,
	)
}

// LinkInfos fetches information about all ethtool-supported links.
func (c *client) LinkInfos() ([]*LinkInfo, error) {
	// Dump info about all links, index 0 and empty name means fetch everything.
	return c.linkInfo(netlink.Dump, 0, "")
}

// LinkInfo fetches information about a single ethtool-supported link.
func (c *client) LinkInfo(r Request) (*LinkInfo, error) {
	if r.Index == 0 && r.Name == "" {
		// The caller has to specify some identifier or the kernel will EINVAL
		// on this path.
		return nil, errBadRequest
	}

	// Request info about a single link. Note that if the client happens to pass
	// an incompatible non-zero index/name pair, the kernel will return an error
	// and we'll immediately send that back.
	lis, err := c.linkInfo(0, r.Index, r.Name)
	if err != nil {
		// If the queried interface is not supported by the ethtool APIs
		// (EOPNOTSUPP) or does not exist at all (ENODEV), enforce the contract
		// with callers by providing an OS-independent Go error which the caller
		// can inspect.
		if errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENODEV) {
			return nil, os.ErrNotExist
		}

		return nil, err
	}

	if l := len(lis); l != 1 {
		// A fundamental request invariant is broken and we can't provide
		// anything of use to the caller.
		panicf("ethtool: unexpected number of LinkInfo messages for request index: %d, name: %q: %d",
			r.Index, r.Name, l)
	}

	return lis[0], nil
}

// linkInfo is the internal shared request function for LinkInfo(s).
func (c *client) linkInfo(flags netlink.HeaderFlags, index int, name string) ([]*LinkInfo, error) {
	// TODO(mdlayher): make this faster by potentially precomputing the byte
	// slice of packed netlink attributes and then modifying the index value at
	// the appropriate byte slice index.
	ae := netlink.NewAttributeEncoder()
	ae.Nested(unix.ETHTOOL_A_LINKINFO_HEADER, func(nae *netlink.AttributeEncoder) error {
		// When fetching by index or name, one or both will be non-zero.
		// Otherwise we leave the header empty to dump all the links.
		if index > 0 {
			nae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, uint32(index))
		}
		if name != "" {
			nae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, name)
		}

		return nil
	})

	// Note: don't send netlink.Acknowledge or we get an extra message back from
	// the kernel which doesn't seem useful as of now.
	msgs, err := c.execute(unix.ETHTOOL_MSG_LINKINFO_GET, flags, ae)
	if err != nil {
		return nil, err
	}

	return parseLinkInfo(msgs)
}

// parseLinkInfo parses LinkInfo structures from a slice of generic netlink
// messages.
func parseLinkInfo(msgs []genetlink.Message) ([]*LinkInfo, error) {
	var lis []*LinkInfo
	for _, m := range msgs {
		ad, err := netlink.NewAttributeDecoder(m.Data)
		if err != nil {
			return nil, err
		}

		var li LinkInfo
		for ad.Next() {
			switch ad.Type() {
			case unix.ETHTOOL_A_LINKINFO_HEADER:
				ad.Nested(parseLinkInfoHeader(&li))
			case unix.ETHTOOL_A_LINKINFO_PORT:
				li.Port = Port(ad.Uint8())
			}
		}

		if err := ad.Err(); err != nil {
			return nil, err
		}

		lis = append(lis, &li)
	}

	return lis, nil
}

// parseLinkInfoHeader decodes information from a link info header into the
// input LinkInfo.
func parseLinkInfoHeader(li *LinkInfo) func(*netlink.AttributeDecoder) error {
	return func(ad *netlink.AttributeDecoder) error {
		for ad.Next() {
			switch ad.Type() {
			case unix.ETHTOOL_A_HEADER_DEV_INDEX:
				li.Index = int(ad.Uint32())
			case unix.ETHTOOL_A_HEADER_DEV_NAME:
				li.Name = ad.String()
			}
		}
		return nil
	}
}

func panicf(format string, a ...interface{}) {
	panic(fmt.Sprintf(format, a...))
}
