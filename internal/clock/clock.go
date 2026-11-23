package clock

import "time"

type Clock interface {
	Now() time.Time
	NewTimer(d time.Duration) *time.Timer
}

type Real struct{}

func (Real) Now() time.Time                       { return time.Now() }
func (Real) NewTimer(d time.Duration) *time.Timer { return time.NewTimer(d) }

type Fake struct {
	NowTime time.Time
}

func (f Fake) Now() time.Time { return f.NowTime }
func (f Fake) NewTimer(d time.Duration) *time.Timer {
	return time.AfterFunc(d, func() {})
}
