package main

import (
	"net"
	"sync/atomic"
	"time"
)

// ¿Hay internet? Se mira cada poco: sin él no se puede jugar.
var redOK atomic.Bool

func init() { redOK.Store(true) }

func hayRed() bool { return redOK.Load() }

func vigilarRed(a *app) {
	for {
		ok := false
		d := net.Dialer{Timeout: 4 * time.Second}
		if c, err := d.Dial("tcp", "lichess.org:443"); err == nil {
			c.Close()
			ok = true
		}
		if redOK.Swap(ok) != ok {
			a.hacer(func() {}) // que se vea el aviso de «sin internet» (o que se quite)
		}
		if ok {
			time.Sleep(30 * time.Second)
		} else {
			time.Sleep(8 * time.Second)
		}
	}
}
