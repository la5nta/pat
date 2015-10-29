package ax25

import (
	"net"
	"testing"
)

// Ref https://github.com/LA5NTA/wl2k-go/issues/10
func TestNilConn(t *testing.T) {
	var conn net.Conn = (*Conn)(nil)
	if conn != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic when calling method on non-nil interface: %v", r)
				}
			}()

			if addr := conn.RemoteAddr(); addr != nil {
				t.Errorf("RemoteAddr returned non-nil value for when called on nil-interface")
			}

			if addr := conn.LocalAddr(); addr != nil {
				t.Errorf("LocalAddr returned non-nil value for when called on nil-interface")
			}
		}()
	}
}
