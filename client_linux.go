//go:build linux
// +build linux

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
	c         *genetlink.Conn
	family    uint16
	monitorID uint32
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

	// Apply quality of life improvement options on a best-effort basis when
	// the kernel is new enough to support those options.
	for _, o := range []netlink.ConnOption{
		netlink.ExtendedAcknowledge,
		netlink.GetStrictCheck,
	} {
		if err := conn.SetOption(o, true); err != nil && !errors.Is(err, unix.ENOPROTOOPT) {
			_ = conn.Close()
			return nil, err
		}
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

	// Find the group ID for the monitor group in case we need it later.
	var monitorID uint32
	for _, g := range f.Groups {
		if g.Name == unix.ETHTOOL_MCGRP_MONITOR_NAME {
			monitorID = g.ID
			break
		}
	}
	if monitorID == 0 {
		return nil, errors.New("ethtool: could not find monitor multicast group ID")
	}

	return &client{
		c:         c,
		family:    f.ID,
		monitorID: monitorID,
	}, nil
}

// Close closes the underlying generic netlink connection.
func (c *client) Close() error { return c.c.Close() }

// LinkInfos fetches information about all ethtool-supported links.
func (c *client) LinkInfos() ([]*LinkInfo, error) {
	return c.linkInfo(netlink.Dump, Interface{})
}

// LinkInfo fetches information about a single ethtool-supported link.
func (c *client) LinkInfo(ifi Interface) (*LinkInfo, error) {
	lis, err := c.linkInfo(0, ifi)
	if err != nil {
		return nil, err
	}

	if l := len(lis); l != 1 {
		panicf("ethtool: unexpected number of LinkInfo messages for request index: %d, name: %q: %d",
			ifi.Index, ifi.Name, l)
	}

	return lis[0], nil
}

// linkInfo is the shared logic for Client.LinkInfo(s).
func (c *client) linkInfo(flags netlink.HeaderFlags, ifi Interface) ([]*LinkInfo, error) {
	msgs, err := c.get(
		unix.ETHTOOL_A_LINKINFO_HEADER,
		unix.ETHTOOL_MSG_LINKINFO_GET,
		flags,
		ifi,
		nil,
	)
	if err != nil {
		return nil, err
	}

	return parseLinkInfo(msgs)
}

// LinkModes fetches modes for all ethtool-supported links.
func (c *client) LinkModes() ([]*LinkMode, error) {
	return c.linkMode(netlink.Dump, Interface{})
}

// LinkMode fetches information about a single ethtool-supported link's modes.
func (c *client) LinkMode(ifi Interface) (*LinkMode, error) {
	lms, err := c.linkMode(0, ifi)
	if err != nil {
		return nil, err
	}

	if l := len(lms); l != 1 {
		panicf("ethtool: unexpected number of LinkMode messages for request index: %d, name: %q: %d",
			ifi.Index, ifi.Name, l)
	}

	return lms[0], nil
}

// linkMode is the shared logic for Client.LinkMode(s).
func (c *client) linkMode(flags netlink.HeaderFlags, ifi Interface) ([]*LinkMode, error) {
	msgs, err := c.get(
		unix.ETHTOOL_A_LINKMODES_HEADER,
		unix.ETHTOOL_MSG_LINKMODES_GET,
		flags,
		ifi,
		nil,
	)
	if err != nil {
		return nil, err
	}

	return parseLinkModes(msgs)
}

// LinkStates fetches link state data for all ethtool-supported links.
func (c *client) LinkStates() ([]*LinkState, error) {
	return c.linkState(netlink.Dump, Interface{})
}

// LinkState fetches link state data for a single ethtool-supported link.
func (c *client) LinkState(ifi Interface) (*LinkState, error) {
	lss, err := c.linkState(0, ifi)
	if err != nil {
		return nil, err
	}

	if l := len(lss); l != 1 {
		panicf("ethtool: unexpected number of LinkState messages for request index: %d, name: %q: %d",
			ifi.Index, ifi.Name, l)
	}

	return lss[0], nil
}

// linkState is the shared logic for Client.LinkState(s).
func (c *client) linkState(flags netlink.HeaderFlags, ifi Interface) ([]*LinkState, error) {
	msgs, err := c.get(
		unix.ETHTOOL_A_LINKSTATE_HEADER,
		unix.ETHTOOL_MSG_LINKSTATE_GET,
		flags,
		ifi,
		nil,
	)
	if err != nil {
		return nil, err
	}

	return parseLinkState(msgs)
}

// WakeOnLANs fetches Wake-on-LAN information for all ethtool-supported links.
func (c *client) WakeOnLANs() ([]*WakeOnLAN, error) {
	return c.wakeOnLAN(netlink.Dump, Interface{})
}

// WakeOnLAN fetches Wake-on-LAN information for a single ethtool-supported
// interface.
func (c *client) WakeOnLAN(ifi Interface) (*WakeOnLAN, error) {
	wols, err := c.wakeOnLAN(0, ifi)
	if err != nil {
		return nil, err
	}

	if l := len(wols); l != 1 {
		panicf("ethtool: unexpected number of WakeOnLAN messages for request index: %d, name: %q: %d",
			ifi.Index, ifi.Name, l)
	}

	return wols[0], nil
}

// SetWakeOnLAN configures Wake-on-LAN parameters for a single ethtool-supported
// interface.
func (c *client) SetWakeOnLAN(wol WakeOnLAN) error {
	_, err := c.get(
		unix.ETHTOOL_A_WOL_HEADER,
		unix.ETHTOOL_MSG_WOL_SET,
		netlink.Acknowledge,
		wol.Interface,
		wol.encode,
	)
	return err
}

// wakeOnLAN is the shared logic for Client.WakeOnLAN(s).
func (c *client) wakeOnLAN(flags netlink.HeaderFlags, ifi Interface) ([]*WakeOnLAN, error) {
	msgs, err := c.get(
		unix.ETHTOOL_A_WOL_HEADER,
		unix.ETHTOOL_MSG_WOL_GET,
		flags,
		ifi,
		nil,
	)
	if err != nil {
		return nil, err
	}

	return parseWakeOnLAN(msgs)
}

// encode packs WakeOnLAN data into the appropriate netlink attributes for the
// encoder.
func (wol WakeOnLAN) encode(ae *netlink.AttributeEncoder) {
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
}

// get performs a request/response interaction with ethtool netlink.
func (c *client) get(
	header uint16,
	cmd uint8,
	flags netlink.HeaderFlags,
	ifi Interface,
	// May be nil; used to apply optional parameters.
	params func(ae *netlink.AttributeEncoder),
) ([]genetlink.Message, error) {
	if flags&netlink.Dump == 0 && ifi.Index == 0 && ifi.Name == "" {
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
		if ifi.Index > 0 {
			nae.Uint32(unix.ETHTOOL_A_HEADER_DEV_INDEX, uint32(ifi.Index))
		}
		if ifi.Name != "" {
			nae.String(unix.ETHTOOL_A_HEADER_DEV_NAME, ifi.Name)
		}

		// Unconditionally add the compact bitsets flag since the ethtool
		// multicast group notifications require the compact format, so we might
		// as well always use it.
		nae.Uint32(unix.ETHTOOL_A_HEADER_FLAGS, unix.ETHTOOL_FLAG_COMPACT_BITSETS)

		return nil
	})

	if params != nil {
		// Optionally apply more parameters to the attribute encoder.
		params(ae)
	}

	// Note: don't send netlink.Acknowledge or we get an extra message back from
	// the kernel which doesn't seem useful as of now.
	msgs, err := c.execute(cmd, flags, ae)
	if err != nil {
		// Unpack the extended acknowledgement error message if possible so the
		// caller doesn't have to unpack it themselves.
		var msg string
		if oerr, ok := err.(*netlink.OpError); ok {
			msg = oerr.Message
		}

		return nil, &Error{
			Message: msg,
			Err:     err,
		}
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

// Is enables Error comparison with sentinel errors that are part of the
// Client's API contract such as os.ErrNotExist and os.ErrPermission.
func (e *Error) Is(target error) bool {
	switch target {
	case os.ErrNotExist:
		// The queried interface is not supported by the ethtool APIs
		// (EOPNOTSUPP) or does not exist at all (ENODEV).
		return errors.Is(e.Err, unix.EOPNOTSUPP) || errors.Is(e.Err, unix.ENODEV)
	case os.ErrPermission:
		// The caller lacks permission to perform an operation.
		return errors.Is(e.Err, unix.EPERM)
	default:
		return false
	}
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
				ad.Nested(parseInterface(&li.Interface))
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
				ad.Nested(parseInterface(&lm.Interface))
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

// parseAdvertisedLinkModes decodes an ethtool compact bitset into the input
// slice of AdvertisedLinkModes.
func parseAdvertisedLinkModes(alms *[]AdvertisedLinkMode) func(*netlink.AttributeDecoder) error {
	return func(ad *netlink.AttributeDecoder) error {
		values, err := newBitset(ad)
		if err != nil {
			return err
		}

		for i, v := range values {
			if v == 0 {
				// No bits set, don't bother checking.
				continue
			}

			// Test each bit to find which ones are set, and use that to look up
			// the proper index in linkModes (accounting for the offset of 32
			// for each value in the array) so we can find the correct link mode
			// to attach. Note that the lookup assumes that there will never be
			// any skipped bits in the linkModes table.
			//
			// Thanks 0x0f10, c_h_lunde, TheCi, and Wacholderbaer from Twitch
			// chat for saving me from myself!
			for j := 0; j < 32; j++ {
				if v&(1<<j) != 0 {
					m := linkModes[(32*i)+j]
					*alms = append(*alms, AdvertisedLinkMode{
						Index: int(m.bit),
						Name:  m.str,
					})
				}
			}
		}

		return nil
	}
}

// parseLinkState parses LinkState structures from a slice of generic netlink
// messages.
func parseLinkState(msgs []genetlink.Message) ([]*LinkState, error) {
	lss := make([]*LinkState, 0, len(msgs))
	for _, m := range msgs {
		ad, err := netlink.NewAttributeDecoder(m.Data)
		if err != nil {
			return nil, err
		}

		var ls LinkState
		for ad.Next() {
			// TODO(mdlayher): try this out on fancier NICs to parse more of the
			// extended state information.
			switch ad.Type() {
			case unix.ETHTOOL_A_LINKSTATE_HEADER:
				ad.Nested(parseInterface(&ls.Interface))
			case unix.ETHTOOL_A_LINKSTATE_LINK:
				// Up/down is reported as a uint8 boolean.
				ls.Link = ad.Uint8() != 0
			}
		}

		if err := ad.Err(); err != nil {
			return nil, err
		}

		lss = append(lss, &ls)
	}

	return lss, nil
}

// parseWakeOnLAN parses WakeOnLAN structures from a slice of generic netlink
// messages.
func parseWakeOnLAN(msgs []genetlink.Message) ([]*WakeOnLAN, error) {
	wols := make([]*WakeOnLAN, 0, len(msgs))
	for _, m := range msgs {
		ad, err := netlink.NewAttributeDecoder(m.Data)
		if err != nil {
			return nil, err
		}

		var wol WakeOnLAN
		for ad.Next() {
			switch ad.Type() {
			case unix.ETHTOOL_A_WOL_HEADER:
				ad.Nested(parseInterface(&wol.Interface))
			case unix.ETHTOOL_A_WOL_MODES:
				ad.Nested(parseWakeOnLANModes(&wol.Modes))
			case unix.ETHTOOL_A_WOL_SOPASS:
				// TODO(mdlayher): parse the password if we can find a NIC that
				// supports it, probably using ad.Bytes.
			}
		}

		if err := ad.Err(); err != nil {
			return nil, err
		}

		wols = append(wols, &wol)
	}

	return wols, nil
}

// parseWakeOnLANModes decodes an ethtool compact bitset into the input WOLMode.
func parseWakeOnLANModes(m *WOLMode) func(*netlink.AttributeDecoder) error {
	return func(ad *netlink.AttributeDecoder) error {
		values, err := newBitset(ad)
		if err != nil {
			return err
		}

		// Assume the kernel will not sprout 25 more Wake-on-LAN modes and just
		// inspect the first uint32 so we can populate the WOLMode bitmask for
		// the caller.
		if l := len(values); l > 1 {
			panicf("ethtool: too many Wake-on-LAN mode uint32s in bitset: %d", l)
		}

		*m = WOLMode(values[0])
		return nil
	}
}

// parseInterface decodes information from a response header into the input
// Interface.
func parseInterface(ifi *Interface) func(*netlink.AttributeDecoder) error {
	return func(ad *netlink.AttributeDecoder) error {
		for ad.Next() {
			switch ad.Type() {
			case unix.ETHTOOL_A_HEADER_DEV_INDEX:
				(*ifi).Index = int(ad.Uint32())
			case unix.ETHTOOL_A_HEADER_DEV_NAME:
				(*ifi).Name = ad.String()
			}
		}
		return nil
	}
}

func panicf(format string, a ...interface{}) {
	panic(fmt.Sprintf(format, a...))
}
