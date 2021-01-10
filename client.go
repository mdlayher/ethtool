package ethtool

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

// LinkInfo contains link settings for an Ethernet interface.
type LinkInfo struct {
	Index int
	Name  string
	Port  Port
}

//go:generate stringer -type=Port -output=string.go

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

// Close cleans up the Client's resources.
func (c *Client) Close() error { return c.c.Close() }
