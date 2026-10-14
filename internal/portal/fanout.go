package portal

import (
	"cmp"
	"context"
	"sync"
)

// fanOut runs calls concurrently and returns the error of the first failed
// call in argument order, nil when every call succeeds. A failed call cancels
// the calls after it; their errors are never returned. Calls before a failed
// call are not cancelled: an error wrapping context.Canceled is not printed.
func fanOut(ctx context.Context, calls ...func(context.Context) error) error {
	ctxs := make([]context.Context, len(calls))
	cancels := make([]context.CancelFunc, len(calls))

	for i := range calls {
		ctxs[i], cancels[i] = context.WithCancel(ctx)
	}

	errs := make([]error, len(calls))

	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Go(func() {
			if errs[i] = call(ctxs[i]); errs[i] != nil {
				for _, cancel := range cancels[i+1:] {
					cancel()
				}
			}
		})
	}

	wg.Wait()

	for _, cancel := range cancels {
		cancel()
	}

	return cmp.Or(errs...)
}
