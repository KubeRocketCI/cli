package portal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewServices_Clock(t *testing.T) {
	t.Parallel()

	before := time.Now()
	assert.False(t, NewServices(nil, Target{}).now().Before(before), "default clock is not time.Now")

	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s := NewServices(nil, Target{}, WithNow(func() time.Time { return fixed }))
	assert.Equal(t, fixed, s.now())
}
