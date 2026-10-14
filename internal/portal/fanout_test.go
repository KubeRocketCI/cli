package portal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errFirst        = errors.New("first")
	errSecond       = errors.New("second")
	errNotCancelled = errors.New("not cancelled")
)

// waitCancelled reports whether ctx is cancelled before a bound that only a
// missing cancellation reaches.
func waitCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	case <-time.After(5 * time.Second):
		return false
	}
}

func TestFanOut_AllSucceed(t *testing.T) {
	t.Parallel()

	ok := func(context.Context) error { return nil }

	require.NoError(t, fanOut(t.Context(), ok, ok, ok))
	require.NoError(t, fanOut(t.Context()))
}

func TestFanOut_ArgumentOrderNotCompletionOrder(t *testing.T) {
	t.Parallel()

	secondDone := make(chan struct{})

	err := fanOut(t.Context(),
		func(context.Context) error {
			<-secondDone

			return errFirst
		},
		func(context.Context) error {
			defer close(secondDone)

			return errSecond
		},
	)

	require.ErrorIs(t, err, errFirst)
}

func TestFanOut_FailureKeepsEarlierCallsRunning(t *testing.T) {
	t.Parallel()

	secondDone := make(chan struct{})

	var firstCtxErr error

	err := fanOut(t.Context(),
		func(ctx context.Context) error {
			<-secondDone

			// The second call returns before fanOut handles its error.
			select {
			case <-ctx.Done():
			case <-time.After(50 * time.Millisecond):
			}

			firstCtxErr = ctx.Err()

			return nil
		},
		func(context.Context) error {
			defer close(secondDone)

			return errSecond
		},
	)

	require.ErrorIs(t, err, errSecond)
	assert.NoError(t, firstCtxErr)
}

func TestFanOut_FailureCancelsLaterCalls(t *testing.T) {
	t.Parallel()

	var secondCancelled, thirdCancelled bool

	err := fanOut(t.Context(),
		func(context.Context) error { return errFirst },
		func(ctx context.Context) error {
			secondCancelled = waitCancelled(ctx)

			return ctx.Err()
		},
		func(ctx context.Context) error {
			thirdCancelled = waitCancelled(ctx)

			return ctx.Err()
		},
	)

	require.ErrorIs(t, err, errFirst)
	assert.True(t, secondCancelled)
	assert.True(t, thirdCancelled)
}

func TestFanOut_ParentCancelReachesEveryCall(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	wait := func(ctx context.Context) error {
		if !waitCancelled(ctx) {
			return errNotCancelled
		}

		return ctx.Err()
	}

	require.ErrorIs(t, fanOut(ctx, wait, wait), context.Canceled)
}
