//nolint:testpackage // tests unexported queue helpers directly
package rqueue

import (
	"testing"
	"time"

	"github.com/gosom/google-maps-scraper/scraper"
	"github.com/stretchr/testify/require"
)

func TestWaitForFlushResultReturnsResultWithinGrace(t *testing.T) {
	ch := make(chan scraper.FlushResult, 1)

	go func() {
		time.Sleep(20 * time.Millisecond)

		ch <- scraper.FlushResult{ResultCount: 3}
	}()

	result, err := waitForFlushResult(ch, 200*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, 3, result.ResultCount)
}

func TestWaitForFlushResultTimesOut(t *testing.T) {
	ch := make(chan scraper.FlushResult)

	_, err := waitForFlushResult(ch, 30*time.Millisecond)
	require.Error(t, err)
	require.Contains(t, err.Error(), "timed out waiting for flush result")
}
