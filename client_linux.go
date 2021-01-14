//+build linux

package ethtool

import (
	"errors"
	"fmt"
	"os"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
	"golang.org/x/sys/unix"
)

// errBadRequest indicates an invalid Request from the caller.
var errBadRequest = errors.New("ethtool: Request must have Index and/or Name set when calling Client methods")

// A client is the Linux implementation backing a Client.
type client struct {
	c      *genetlink.Conn
	family uint16
}

// Note that some Client methods may panic if the kernel returns an unexpected
// number of netlink messages when only one is expected. This means that a
// fundamental request invariant is broken and we can't provide anything of use
// to the caller, so a panic seems reasonable.

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

// LinkInfos fetches information about all ethtool-supported links.
func (c *client) LinkInfos() ([]*LinkInfo, error) {
	return c.linkInfo(netlink.Dump, Request{})
}

// LinkInfo fetches information about a single ethtool-supported link.
func (c *client) LinkInfo(r Request) (*LinkInfo, error) {
	lis, err := c.linkInfo(0, r)
	if err != nil {
		return nil, err
	}

	if l := len(lis); l != 1 {
		panicf("ethtool: unexpected number of LinkInfo messages for request index: %d, name: %q: %d",
			r.Index, r.Name, l)
	}

	return lis[0], nil
}

// linkInfo is the shared logic for Client.LinkInfo(s).
func (c *client) linkInfo(flags netlink.HeaderFlags, r Request) ([]*LinkInfo, error) {
	msgs, err := c.get(
		unix.ETHTOOL_A_LINKINFO_HEADER,
		unix.ETHTOOL_MSG_LINKINFO_GET,
		flags,
		r,
	)
	if err != nil {
		return nil, err
	}

	return parseLinkInfo(msgs)
}

// LinkModes fetches modes for all ethtool-supported links.
func (c *client) LinkModes() ([]*LinkMode, error) {
	return c.linkMode(netlink.Dump, Request{})
}

// LinkMode fetches information about a single ethtool-supported link's modes.
func (c *client) LinkMode(r Request) (*LinkMode, error) {
	lms, err := c.linkMode(0, r)
	if err != nil {
		return nil, err
	}

	if l := len(lms); l != 1 {
		panicf("ethtool: unexpected number of LinkMode messages for request index: %d, name: %q: %d",
			r.Index, r.Name, l)
	}

	return lms[0], nil
}

// linkMode is the shared logic for Client.LinkMode(s).
func (c *client) linkMode(flags netlink.HeaderFlags, r Request) ([]*LinkMode, error) {
	msgs, err := c.get(
		unix.ETHTOOL_A_LINKMODES_HEADER,
		unix.ETHTOOL_MSG_LINKMODES_GET,
		flags,
		r,
	)
	if err != nil {
		return nil, err
	}

	return parseLinkModes(msgs)
}

// get performs a read-only request to ethtool netlink and enforces some of the
// API contracts regarding os.ErrNotExist.
func (c *client) get(
	header uint16,
	cmd uint8,
	flags netlink.HeaderFlags,
	r Request,
) ([]genetlink.Message, error) {
	if flags&netlink.Dump == 0 && r.Index == 0 && r.Name == "" {
		// The caller is not requesting to dump information for multiple
		// interfaces and thus has to specify some identifier or the kernel will
		// EINVAL on this path.
		return nil, errBadRequest
	}

	// TODO(mdlayher): make this faster by potentially precomputing the byte
	// slice of packed netlink attributes and then modifying the index value at
	// the appropriate byte slice index.
	ae := netlink.NewAttributeEncoder()
	ae.Nested(header, func(nae *netlink.AttributeEncoder) error {
		// When fetching by index or name, one or both will be non-zero.
		// Otherwise we leave the header empty to dump all the links.
		//
		// Note that if the client happens to pass an incompatible non-zero
		// index/name pair, the kernel will return an error and we'll
		// immediately send that back.
		if r.Index > 0 {
			nae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, uint32(r.Index))
		}
		if r.Name != "" {
			nae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, r.Name)
		}

		// Unconditionally add the compact bitsets flag since the ethtool
		// multicast group notifications require the compact format, so we might
		// as well always use it.
		nae.Uint32(unix.ETHTOOL_A_HEADER_FLAGS, unix.ETHTOOL_FLAG_COMPACT_BITSETS)

		return nil
	})

	// Note: don't send netlink.Acknowledge or we get an extra message back from
	// the kernel which doesn't seem useful as of now.
	msgs, err := c.execute(cmd, flags, ae)
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

	return msgs, nil
}

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

// parseLinkInfo parses LinkInfo structures from a slice of generic netlink
// messages.
func parseLinkInfo(msgs []genetlink.Message) ([]*LinkInfo, error) {
	lis := make([]*LinkInfo, 0, len(msgs))
	for _, m := range msgs {
		ad, err := netlink.NewAttributeDecoder(m.Data)
		if err != nil {
			return nil, err
		}

		var li LinkInfo
		for ad.Next() {
			switch ad.Type() {
			case unix.ETHTOOL_A_LINKINFO_HEADER:
				ad.Nested(parseHeader(&li.Index, &li.Name))
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

// parseLinkModes parses LinkMode structures from a slice of generic netlink
// messages.
func parseLinkModes(msgs []genetlink.Message) ([]*LinkMode, error) {
	lms := make([]*LinkMode, 0, len(msgs))
	for _, m := range msgs {
		ad, err := netlink.NewAttributeDecoder(m.Data)
		if err != nil {
			return nil, err
		}

		var lm LinkMode
		for ad.Next() {
			switch ad.Type() {
			case unix.ETHTOOL_A_LINKMODES_HEADER:
				ad.Nested(parseHeader(&lm.Index, &lm.Name))
			case unix.ETHTOOL_A_LINKMODES_OURS:
				ad.Nested(parseAdvertisedLinkModes(&lm.Ours))
			case unix.ETHTOOL_A_LINKMODES_PEER:
				ad.Nested(parseAdvertisedLinkModes(&lm.Peer))
			case unix.ETHTOOL_A_LINKMODES_SPEED:
				lm.SpeedMegabits = int(ad.Uint32())
			case unix.ETHTOOL_A_LINKMODES_DUPLEX:
				lm.Duplex = Duplex(ad.Uint8())
			}
		}

		if err := ad.Err(); err != nil {
			return nil, err
		}

		lms = append(lms, &lm)
	}

	return lms, nil
}

// parseAdvertisedLinkModes parses the compact nested attribute format for a
// slice of AdvertisedLinkModes.
func parseAdvertisedLinkModes(alms *[]AdvertisedLinkMode) func(*netlink.AttributeDecoder) error {
	return func(ad *netlink.AttributeDecoder) error {
		// Begin iterating the outer netlink array by its values.
		var values, mask bitfield32
		for ad.Next() {
			switch ad.Type() {
			case unix.ETHTOOL_A_BITSET_SIZE:
				// TODO(mdlayher): consider capping the number of valid bits or
				// similar based on what the kernel tells us here.
			case unix.ETHTOOL_A_BITSET_VALUE:
				ad.Do(parseBitfield32(&values))
			case unix.ETHTOOL_A_BITSET_MASK:
				ad.Do(parseBitfield32(&mask))
			}
		}

		// Do a quick check for errors before making use of the bitfield32s.
		if err := ad.Err(); err != nil {
			return err
		}

		// Only apply modes which exist after the mask is applied. Bits in the
		// values bitmap will match link mode bits from linkModes.
		values.Value &= mask.Value
		for _, m := range linkModes {
			if values.Value&(1<<m.bit) != 0 {
				*alms = append(*alms, AdvertisedLinkMode{
					Index: int(m.bit),
					Name:  m.str,
				})
			}
		}

		return nil
	}
}

// parseHeader decodes information from a response header into the input
// values.
func parseHeader(index *int, name *string) func(*netlink.AttributeDecoder) error {
	return func(ad *netlink.AttributeDecoder) error {
		for ad.Next() {
			switch ad.Type() {
			case unix.ETHTOOL_A_HEADER_DEV_INDEX:
				*index = int(ad.Uint32())
			case unix.ETHTOOL_A_HEADER_DEV_NAME:
				*name = ad.String()
			}
		}
		return nil
	}
}

// TODO(mdlayher): consider moving this with convenience methods to package
// netlink.

// A bitfield32 is a NLA_BITFIELD32 structure.
type bitfield32 struct {
	Value, Selector uint32
}

// parseBitfield32 parses a bitfield32 structure from raw bytes.
func parseBitfield32(bf *bitfield32) func([]byte) error {
	return func(b []byte) error {
		// TODO(mdlayher): what are the last 4 bytes? Netlink padding?
		if len(b) != 12 {
			return errors.New("bitfield32 must contain exactly 12 bytes")
		}

		bf.Value = nlenc.Uint32(b[0:4])
		bf.Selector = nlenc.Uint32(b[4:8])
		return nil
	}
}

func panicf(format string, a ...interface{}) {
	panic(fmt.Sprintf(format, a...))
}
