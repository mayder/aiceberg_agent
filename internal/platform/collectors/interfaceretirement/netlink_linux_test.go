package interfaceretirement

import (
	"context"
	"encoding/binary"
	"net"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"golang.org/x/sys/unix"
)

func TestDecodeRemovedLinkAcceptsOnlyDeleteWithHardwareAddress(t *testing.T) {
	raw := make([]byte, unix.NLMSG_HDRLEN+unix.SizeofIfInfomsg+12)
	binary.NativeEndian.PutUint32(raw[:4], uint32(len(raw)))
	binary.NativeEndian.PutUint16(raw[4:6], unix.RTM_DELLINK)
	attr := raw[unix.NLMSG_HDRLEN+unix.SizeofIfInfomsg:]
	binary.NativeEndian.PutUint16(attr, 10)
	binary.NativeEndian.PutUint16(attr[2:], unix.IFLA_ADDRESS)
	copy(attr[4:], []byte{2, 0, 0, 0, 0, 9})
	now := time.Unix(1800000000, 0)
	got := decodeRemovedLinks(raw, now)
	if len(got) != 1 || got[0].mac.String() != "02:00:00:00:00:09" || got[0].observed != now {
		t.Fatalf("invalid notification result: %+v", got)
	}
	binary.NativeEndian.PutUint16(raw[4:6], unix.RTM_NEWLINK)
	if got := decodeRemovedLinks(raw, now); len(got) != 0 {
		t.Fatal("an update or rename is not a removal")
	}
	binary.NativeEndian.PutUint16(raw[4:6], unix.RTM_DELLINK)
	binary.NativeEndian.PutUint16(attr, 50)
	if got := decodeRemovedLinks(raw, now); len(got) != 0 {
		t.Fatal("a truncated hardware address is not evidence")
	}
}

func TestKernelInterfaceRemovalInIsolatedLinux(t *testing.T) {
	if os.Getenv("AICEBERG_TEST_DISPOSABLE_LINK_NAMESPACE") != "1" {
		t.Skip("requires a disposable Linux network namespace")
	}
	// A host interface is never used. The test creates its own veth pair.
	const name = "aice_disc_a"
	runIP := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("ip", args...).CombinedOutput(); err != nil {
			t.Fatalf("ip %v: %v: %s", args, err, out)
		}
	}
	runIP("link", "add", name, "type", "veth", "peer", "name", "aice_disc_b")
	t.Cleanup(func() { _ = exec.Command("ip", "link", "delete", name).Run() })
	iface, err := net.InterfaceByName(name)
	if err != nil {
		t.Fatal(err)
	}
	mac := iface.HardwareAddr.String()
	runIP("link", "set", "dev", name, "up")
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	messages := make(chan monitoring.DiscoveryEvent, 8)
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, 3, 17, func(_ context.Context, events []monitoring.DiscoveryEvent) error {
			for _, event := range events {
				select {
				case messages <- event:
				default:
				}
			}
			return nil
		})
	}()
	// The socket is opened asynchronously. Wait until this process owns a
	// subscribed socket by checking the route netlink proc table.
	waitForLinkSubscription(t, ctx)
	runIP("link", "set", "dev", name, "down")
	select {
	case <-messages:
		t.Fatal("administrative down emitted a retirement")
	case <-time.After(100 * time.Millisecond):
	}
	runIP("link", "delete", name)
	want, _ := monitoring.NewDiscoveryEvent(3, 17, "interfaces", []string{mac}, "tombstone", 100, 300, time.Now())
	for {
		select {
		case <-ctx.Done():
			t.Fatal("kernel removal did not reach the discovery producer")
		case err := <-done:
			t.Fatalf("watcher exited early: %v", err)
		case event := <-messages:
			if event.Fingerprint == want.Fingerprint {
				stop()
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}
}

func waitForLinkSubscription(t *testing.T, ctx context.Context) {
	t.Helper()
	for ctx.Err() == nil {
		fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
		if err != nil {
			t.Fatal(err)
		}
		err = unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK, Pid: uint32(os.Getpid())})
		_ = unix.Close(fd)
		if err == syscall.EADDRINUSE {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("watcher did not subscribe")
}
