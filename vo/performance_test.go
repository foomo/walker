package vo

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBucketListIndex(t *testing.T) {
	l := GetBucketList()
	assert.Equal(t, 0, l.Index(0), "zero is awesome")
	assert.Equal(t, 0, l.Index(49*time.Millisecond))
	assert.Equal(t, 1, l.Index(50*time.Millisecond), "exact boundaries belong to the upper bucket")
	assert.Equal(t, len(l)-1, l.Index(10*time.Second))
	assert.Equal(t, len(l)-1, l.Index(2*time.Hour), "beyond the last bucket")
}

func TestPerformance(t *testing.T) {
	results := map[string]ScrapeResult{}
	add := func(group string, durations ...time.Duration) {
		for _, d := range durations {
			results[group+strconv.Itoa(len(results))] = ScrapeResult{Group: group, Duration: d}
		}
	}
	ms := time.Millisecond
	add("product", 10*ms, 20*ms, 30*ms, 40*ms, 50*ms, 60*ms, 70*ms, 80*ms, 90*ms, 2*time.Second)
	add("content", 0)

	performance := Performance(results)
	if assert.Len(t, performance, 2) {
		assert.Equal(t, "content", performance[0].Group, "sorted by group")
		product := performance[1]
		assert.Equal(t, 10, product.Pages)
		assert.Len(t, product.Buckets, len(GetBucketList()), "all buckets, empty ones too")
		assert.Equal(t, 4, product.Buckets[0].Pages, "< 50ms")
		assert.Equal(t, 5, product.Buckets[1].Pages, "50ms - 100ms")
		assert.Equal(t, 1, product.Buckets[6].Pages, "1s - 3s")
		assert.Equal(t, 50*ms, product.P50)
		assert.Equal(t, 90*ms, product.P90)
		assert.Equal(t, 2*time.Second, product.Max)

		total := 0
		for _, b := range product.Buckets {
			total += b.Pages
		}
		assert.Equal(t, product.Pages, total)
	}
	assert.Empty(t, Performance(nil))
}
