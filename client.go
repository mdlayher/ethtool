package ethtool

//go:generate stringer -type=Duplex,Port -output=string.go

// A Client can manipulate the ethtool netlink interface.
type Client struct {
	// The operating system-specific client.
	c *client
}

// New creates a Client which can issue ethtool commands.
func New() (*Client, error) {
	c, err := newClient()
	if err != nil {
		return nil, err
	}

	return &Client{c: c}, nil
}

// A Request is the ethtool netlink interface request header, which is used to
// identify an interface being queried by its index and/or name.
type Request struct {
	// Callers may choose to set either Index, Name, or both fields. Note that
	// if both are set, the kernel will verify that both Index and Name are
	// associated with the same interface. If they are not, an error will be
	// returned.
	Index int
	Name  string
}

// LinkInfo contains link settings for an Ethernet interface.
type LinkInfo struct {
	Index int
	Name  string
	Port  Port
}

// A Port is the port type for a LinkInfo structure.
type Port int

// Possible Port type values.
const (
	TwistedPair  Port = 0x00
	AUI          Port = 0x01
	MII          Port = 0x02
	Fibre        Port = 0x03
	BNC          Port = 0x04
	DirectAttach Port = 0x05
	None         Port = 0xef
	Other        Port = 0xff
)

// LinkInfos fetches LinkInfo structures for each ethtool-supported interface
// on this system.
func (c *Client) LinkInfos() ([]*LinkInfo, error) {
	return c.c.LinkInfos()
}

// LinkInfo fetches LinkInfo for the interface specified by the Request header.
//
// If the requested device does not exist or is not supported by the ethtool
// interface, an error compatible with errors.Is(err, os.ErrNotExist) will be
// returned.
func (c *Client) LinkInfo(r Request) (*LinkInfo, error) {
	return c.c.LinkInfo(r)
}

// LinkMode contains link mode information for an Ethernet interface.
type LinkMode struct {
	Index         int
	Name          string
	SpeedMegabits int
	Ours, Peer    []AdvertisedLinkMode
	Duplex        Duplex
}

// A Duplex is the link duplex type for a LinkMode structure.
type Duplex int

// Possible Duplex type values.
const (
	Half    Duplex = 0x00
	Full    Duplex = 0x01
	Unknown Duplex = 0xff
)

// An AdvertisedLinkMode is a link mode that an interface advertises it is
// capable of using.
type AdvertisedLinkMode struct {
	Index int
	Name  string
}

// LinkModes fetches LinkMode structures for each ethtool-supported interface
// on this system.
func (c *Client) LinkModes() ([]*LinkMode, error) {
	return c.c.LinkModes()
}

// LinkMode fetches LinkMode data for the interface specified by the Request
// header.
//
// If the requested device does not exist or is not supported by the ethtool
// interface, an error compatible with errors.Is(err, os.ErrNotExist) will be
// returned.
func (c *Client) LinkMode(r Request) (*LinkMode, error) {
	return c.c.LinkMode(r)
}

// Close cleans up the Client's resources.
func (c *Client) Close() error { return c.c.Close() }
