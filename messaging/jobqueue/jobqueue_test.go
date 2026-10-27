package jobqueue_test

import (
	"testing"
	"time"

	"github.com/nostosx/servex/messaging/jobqueue"
)

func TestApplyOptions(t *testing.T) {
	tests := []struct {
		name string
		opts []jobqueue.EnqueueOption
		want jobqueue.EnqueueOptions
	}{
		{"defaults", nil, jobqueue.EnqueueOptions{MaxRetry: 3}},
		{"nil option skipped", []jobqueue.EnqueueOption{nil}, jobqueue.EnqueueOptions{MaxRetry: 3}},
		{"delay", []jobqueue.EnqueueOption{jobqueue.WithDelay(time.Minute)},
			jobqueue.EnqueueOptions{Delay: time.Minute, MaxRetry: 3}},
		{"negative delay", []jobqueue.EnqueueOption{jobqueue.WithDelay(-time.Second)},
			jobqueue.EnqueueOptions{MaxRetry: 3}},
		{"max retry", []jobqueue.EnqueueOption{jobqueue.WithMaxRetry(7)}, jobqueue.EnqueueOptions{MaxRetry: 7}},
		{"zero max retry", []jobqueue.EnqueueOption{jobqueue.WithMaxRetry(0)}, jobqueue.EnqueueOptions{}},
		{"negative max retry", []jobqueue.EnqueueOption{jobqueue.WithMaxRetry(-1)}, jobqueue.EnqueueOptions{}},
		{"unique", []jobqueue.EnqueueOption{jobqueue.WithUnique("k", time.Hour)},
			jobqueue.EnqueueOptions{MaxRetry: 3, UniqueKey: "k", UniqueTTL: time.Hour}},
		{"later options win", []jobqueue.EnqueueOption{jobqueue.WithMaxRetry(1), jobqueue.WithMaxRetry(5)},
			jobqueue.EnqueueOptions{MaxRetry: 5}},
		{"all", []jobqueue.EnqueueOption{
			jobqueue.WithDelay(time.Second), jobqueue.WithMaxRetry(2), jobqueue.WithUnique("u", time.Minute),
		}, jobqueue.EnqueueOptions{Delay: time.Second, MaxRetry: 2, UniqueKey: "u", UniqueTTL: time.Minute}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jobqueue.ApplyOptions(tt.opts...); got != tt.want {
				t.Fatalf("ApplyOptions() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestErrDuplicate(t *testing.T) {
	if got, want := jobqueue.ErrDuplicate.Error(), "jobqueue: duplicate job"; got != want {
		t.Fatalf("ErrDuplicate = %q, want %q", got, want)
	}
}
