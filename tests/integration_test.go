package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/its-ernest/rundown-workers/pkg/engine"
	"github.com/stretchr/testify/assert"
)

const baseURL = "http://localhost:8181"

func TestIntegration(t *testing.T) {
	queue := "test_queue"
	payload := "test-payload-123"

	// 1. Enqueue
	t.Run("Enqueue", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"queue":   queue,
			"payload": payload,
		})
		resp, err := http.Post(baseURL+"/enqueue", "application/json", bytes.NewBuffer(body))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var job engine.Job
		json.NewDecoder(resp.Body).Decode(&job)
		assert.NotEmpty(t, job.ID)
		assert.Equal(t, queue, job.Queue)
		assert.Equal(t, payload, job.Payload)
		assert.Equal(t, engine.StatusPending, job.Status)
	})

	time.Sleep(50 * time.Millisecond)

	// 2. Poll
	var polledJob engine.Job
	t.Run("Poll", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"queue": queue,
		})
		resp, err := http.Post(baseURL+"/poll", "application/json", bytes.NewBuffer(body))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		json.NewDecoder(resp.Body).Decode(&polledJob)
		assert.NotEmpty(t, polledJob.ID)
		assert.Equal(t, engine.StatusRunning, polledJob.Status)
	})

	// 3. Complete
	t.Run("Complete", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"id": polledJob.ID,
		})
		resp, err := http.Post(baseURL+"/complete", "application/json", bytes.NewBuffer(body))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// 4. Poll again (empty)
	t.Run("PollEmpty", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"queue": queue,
		})
		resp, err := http.Post(baseURL+"/poll", "application/json", bytes.NewBuffer(body))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})
}
