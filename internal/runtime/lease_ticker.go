package runtime

import "time"

// LeaseTicker belongs to the runtime layer. It keeps heartbeat renewal
// deterministic in tests without turning the worker into a separate actor
// system.
type LeaseTicker interface {
	C() <-chan time.Time
	Stop()
}

type LeaseTickerFactory interface {
	New(interval time.Duration) LeaseTicker
}

type systemLeaseTickerFactory struct{}

func (systemLeaseTickerFactory) New(interval time.Duration) LeaseTicker {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return systemLeaseTicker{ticker: time.NewTicker(interval)}
}

type systemLeaseTicker struct {
	ticker *time.Ticker
}

func (t systemLeaseTicker) C() <-chan time.Time {
	return t.ticker.C
}

func (t systemLeaseTicker) Stop() {
	t.ticker.Stop()
}

type manualLeaseTicker struct {
	ch chan time.Time
}

func newManualLeaseTicker() *manualLeaseTicker {
	return &manualLeaseTicker{ch: make(chan time.Time, 16)}
}

func (t *manualLeaseTicker) C() <-chan time.Time {
	return t.ch
}

func (t *manualLeaseTicker) Stop() {}

func (t *manualLeaseTicker) Tick(at time.Time) {
	t.ch <- at.UTC()
}

type staticLeaseTickerFactory struct {
	ticker LeaseTicker
}

func (f staticLeaseTickerFactory) New(_ time.Duration) LeaseTicker {
	if f.ticker != nil {
		return f.ticker
	}
	return systemLeaseTickerFactory{}.New(10 * time.Second)
}
