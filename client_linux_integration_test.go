//go:build linux
// +build linux

package ethtool_test

import (
	"errors"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mdlayher/ethtool"
	"github.com/mdlayher/netlink"
)

func TestIntegrationClientLinkInfos(t *testing.T) {
	// Make sure the basic netlink plumbing is in place without requiring the
	// use of any privileged operations.
	c, err := ethtool.New()
	if err != nil {
		t.Fatalf("failed to open client: %v", err)
	}
	defer c.Close()

	lis, err := c.LinkInfos()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Skipf("skipping, operation not supported: %v", err)
		}

		t.Fatalf("failed to fetch link infos: %v", err)
	}

	for _, li := range lis {
		t.Logf("%d: %q: %s", li.Interface.Index, li.Interface.Name, li.Port)
	}
}

func TestIntegrationClientNetlinkStrict(t *testing.T) {
	c, err := ethtool.New()
	if err != nil {
		t.Fatalf("failed to open client: %v", err)
	}

	_, err = c.LinkInfo(ethtool.Interface{Name: "notexist0"})
	if err == nil {
		t.Fatal("expected an error, but none occurred")
	}

	var eerr *ethtool.Error
	if !errors.As(err, &eerr) {
		t.Fatalf("expected outer wrapped *ethtool.Error, but got: %T", err)
	}

	// The underlying error is *netlink.OpError but for the purposes of this
	// test, all we really care about is that the kernel produced an error
	// message regarding a non-existent device. This means that the netlink
	// Strict option plumbing is working.
	var nerr *netlink.OpError
	if !errors.As(eerr, &nerr) {
		t.Fatalf("expected inner wrapped *netlink.OpError, but got: %T", eerr)
	}
	eerr.Err = nil

	want := &ethtool.Error{Message: "no device matches name"}
	if diff := cmp.Diff(want, eerr); diff != "" {
		t.Fatalf("unexpected *ethtool.Error (-want +got):\n%s", diff)
	}
}
