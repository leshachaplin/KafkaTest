package dispatcher

import (
	"context"
)

type Operation interface {
}

type Dispatcher struct {
	Operation   chan Operation
	CancelRead  chan Operation
	CancelWrite chan Operation
	OnOperation func(operation Operation)
}

func Do(d *Dispatcher, done context.Context) {
	go func(d *Dispatcher) {
		for {
			select {
			case m := <-d.Operation:
				d.OnOperation(m)
			case <-done.Done():
				return
			}
		}
	}(d)
}
