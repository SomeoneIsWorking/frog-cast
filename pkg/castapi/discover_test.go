package castapi

import (
	"context"
	"testing"

	"github.com/grandcat/zeroconf"
)

// The desktop advertises with zeroconf; a unicast browse must find it.
func TestDiscoverFindsAZeroconfAdvertisement(t *testing.T) {
	server, err := zeroconf.Register("test instance", "_wailscast._tcp", "local.", 45123, []string{"app=wails-cast"}, nil)
	if err != nil {
		t.Skipf("can't advertise here: %v", err)
	}
	defer server.Shutdown()

	found, err := discover(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, inst := range found {
		if inst.Name == "test instance" {
			if inst.Port != 45123 || inst.Host == "" {
				t.Fatalf("instance = %+v", inst)
			}
			return
		}
	}
	t.Fatalf("test instance not found in %+v", found)
}
