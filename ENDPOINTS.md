# HTTP Protocol endpoints

All endpoints use JSON documents. The contents of these documents can be found
in `flamenco/documents.go`.


## Exposed to Flamenco Worker

There are a few standard responses (`404 Not Found` for task updates on a
non-existant task ID, for example) that are not covered here.

### Authentication

Workers register themselves with a random secret at the `/register-worker`
endpoint. The Manager will respond with the worker information, including its
ID. Subsequent calls must then include a Basic HTTP authorization header with
the worker ID as username and the random secret as password.

### `/register-worker`

Expects a `POST` with a `WorkerRegistration` document.

Returns `Worker` document, which contains a field `_id` to be used as username
in authenticated calls.

Allows workers to register themselves with a nickname, secret (used to
authenticate subsequent calls), supported task types, and more info.

### `/sign-on`

Expects an authenticated `POST` with a `WorkerSignonDoc` document.

This gives the worker the possibility to change its nickname and supported task
types, without having to re-register. It also marks the worker status as
`starting`.

### `/sign-off`

Expects an authenticated `POST`. Re-queues any task that was assigned to the
worker, so that they can be assigned to a different worker.

### `/task`

Expects an authenticated `GET`.

Can return different responses with different HTTP status codes:

- `200 OK`: Indicates the worker should execute a task. Returns a `Task`
  document.
- `204 No Content`: Indicates that there are no tasks to perform at this moment.
- `406 Not Acceptable`: Indicates that the worker has no supported tasks types.
- `423 Locked`: Indicates that the worker should not perform a task at this
  moment, but rather change its status. Returns a `WorkerStatus` document with
  the requested status.

### `/tasks/{task-id}/update`

Expects an authenticated `POST` with a `TaskUpdate` document.

Can return different responses with different HTTP status codes:

- `204 No Content`: The update was accepted.
- `409 Conflict`: The task is assigned to another worker, so the update was not
  accepted.

### `/logfile/{job-id}/{task-id}`

Expects a `GET`.

Serves the latest log file for that task. Rotated log files cannot be accessed
via any URL.

### `/may-i-run/{task-id}`

Expectes an authenticated `GET`.

Returns a `MayKeepRunningResponse` document indicating whether the worker is
allowed to run / keep running the task, and possibly any queued requested worker
status change.

### `/status-change`

Expectes an authenticated `GET`.

Can return different responses with different HTTP status codes:

- `200 OK`: Returns a `WorkerStatus` document with the status to change to.
- `204 No Content`: No status change is queued for the worker.

### `/ack-status-change/{ack-status}`

Expectes an authenticated `POST`. The URL should contain the status that is
being acknowledged (or otherwise changed to). Acknowledging a status that wasn't
queued as status change for the worker is accepted, but will trigger a warning
in the Manager's log.

Returns a `204 No Content`.

### `/output-produced`

Expects an authenticated `POST` with a `FileProduced` document.

This is used by workers to indicate to the Manager that an image was produced
that should be shown as 'latest image' in the dashboard.

- `204 No Content`: The update was accepted. Note that this does not ensure
  display on the dashboard; when the queue of images to show is too large, new
  updates will be dropped and a warning logged.
- `422 Unprocessable Entity`: The request did not contain any image paths.


## Used on Flamenco Server

To be documented.
