// Package jobqueue defines the background job contract of servex: enqueueing
// jobs with delay, retry and uniqueness options, and a worker Hook that runs
// job handlers within the application lifecycle.
//
// Job handlers are plain endpoint.Endpoint values whose request is a *Job.
// Returning nil completes a job; a retryable error (see errorx.IsRetryable)
// schedules another attempt with backoff until EnqueueOptions.MaxRetry is
// exhausted; any other error fails the job immediately.
package jobqueue
