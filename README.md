# Rundown-Workers

Rundown-Workers is a lightweight, language-agnostic workflow executor for developers who need reliable background job processing without heavy infrastructure.

It combines a Go-based core engine with simple SDKs (Python, Node.js, etc.), allowing tasks to be defined and executed in any language.

---

## Philosophy

Most workflow systems are powerful but unnecessarily complex.

Rundown-Workers follows a simpler principle:

Keep the engine minimal and let execution happen in the language where the code already lives.

* The engine orchestrates
* The SDK executes

---

## Architecture Overview

The system consists of:

* A Go engine (HTTP API + SQLite)
* Language-specific SDKs (Python, Node.js, Go, etc.)
* Workers that poll and execute jobs

Communication happens over HTTP, making the system language-agnostic.

---

## How It Works

1. A job is enqueued into the engine
2. A worker polls the engine for jobs
3. The engine assigns a job
4. The SDK executes the job locally
5. The worker reports completion

---

## Installation

### 1. Clone repository

```
git clone https://github.com/yourusername/rundown-workers.git
cd rundown-workers
```

### 2. Run the engine

```
go run cmd/worker/main.go
```

The server starts at:

```
http://localhost:8181
```

---

## Basic Usage

### Step 1 — Define a Worker (Python)

```python
import rundown_workers as rw

@rw.queue(name="post_worker", host="http://localhost:8181")
def run_work(payload):
    print("Processing:", payload)
```

Run the worker:

```
python worker.py
```

This starts a background process that continuously polls for jobs.

---

### Step 2 — Enqueue a Job

```
curl -X POST http://localhost:8181/enqueue \
-H "Content-Type: application/json" \
-d '{
  "queue": "post_worker",
  "payload": "Hello from Rundown"
}'
```

---

## What Happens Next

* The engine stores the job in SQLite
* The worker polls the /poll endpoint
* A job is assigned
* The function executes
* The worker calls /complete
* The job is marked as done

---

## Core Concepts

### Queue

A named channel for jobs.

Examples:

```
post_worker
email_sender
image_processor
```

---

### Job

A unit of work:

```
{
  "id": "uuid",
  "queue": "post_worker",
  "payload": "data",
  "status": "pending"
}
```

---

### Worker

A process that:

* Polls the engine
* Executes jobs
* Reports completion

---

## Job Lifecycle

```
pending -> running -> done
```

---

## Why This Design

Rundown-Workers avoids forcing all logic into a single language.

It allows:

* Python for data processing
* Node.js for asynchronous tasks
* Go for performance-sensitive work

All coordinated through a single lightweight engine.

---

## Current Limitations

This project is in an early stage.

Missing features include:

* Retry mechanism
* Job timeouts
* Dead letter queue
* Scheduling or delayed jobs
* Authentication
* Observability (logging and metrics)

---

## Roadmap

* Retry and backoff strategy
* Timeout handling and job recovery
* Multiple workers per queue
* SDK for Node.js
* CLI tooling
* Monitoring dashboard
* Optional distributed mode (e.g. PostgreSQL)

---

## Contributing

Contributions are welcome.

Areas to start:

* SDK improvements
* Retry logic
* Testing

---

## License

MIT

---

## Final Note

Rundown-Workers is not designed to compete with complex workflow engines.

It is built for simplicity, clarity, and control.

Simple systems scale better because they are easier to reason about and harder to break.
