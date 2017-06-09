# Flamenco Manager

This is the Flamenco Manager implementation in Go.

Author: Sybren A. Stüvel <sybren@blender.studio>


## Getting started

Install [MongoDB 3.2 or newer](https://docs.mongodb.com/manual/administration/install-community/),
copy `flamenco-manager-example.yaml` to `flamenco-manager.yaml` and edit the file to suit your needs
(see below), then start Flamenco Manager. Connect a browser, and you should see a (probably empty)
status dashboard.

## Configuration

This describes the minimal changes you'll have to do to get Flamenco Manager running.

- Copy `flamenco-manager-example.yaml` to `flamenco-manager.yaml` if you haven't done that yet.
- Update `own_url` to point to the IP address or hostname by which your machine can be reached
  by the workers.
- Set the `manager_id` and `manager_secret` to the values obtained from the Blender Cloud
  administrators.
- Either generate TLS certificates (TLS is the we-are-no-longer-living-in-the-90ies-name for
  SSL), or remove the `tlskey` and `tlscert` options from your `flamenco-manager.yaml` file.
- Update the `variables` for your render farm. The `blender` variable should point to the
  Blender executable where it can be found *on the workers*.
- Update the `path_replacement` variables for your render farm. This allows you to set different
  paths for both Clients (like the Blender Cloud Add-on) and Workers, given their respective
  platforms.

Note that `variables` and `path_replacement` share a namespace -- variable names have to be
unique, and cannot be used in both `variables` and `path_replacement` sections. If this happens,
Flamenco Manager will log the offending name, and refuse to start.


## CLI arguments

Flamenco Manager accepts the following CLI arguments:

- `-debug`: Enable debug-level logging
- `-verbose`: Enable info-level logging (no-op if `-debug` is also given)
- `-json`: Log in JSON format, instead of plain text
- `-cleanslate`: Start with a clean slate; erases all tasks from the local MongoDB,
  then exits Flamenco Manager. This can be run while another Flamenco Manager is
  running, but this scenario has not been well-tested yet.


## Running as service via systemd (Linux-only)

1. Build and configure Flamenco Manager (see below).
2. Edit `flamenco-manager.service` to update it for the installation location, then place the file
   in `/etc/systemd/system`.
3. Run `systemctl daemon-reload` to pick up on the new/edited file.
4. Run `systemctl start flamenco-manager` to start Flamenco Manager.
5. Run `systemctl enable flamenco-manager` to ensure it starts at boot too.


## Starting development

`$FM` denotes the directory containing a checkout of Flamenco Manager, that is, the absolute path
of this `flamenco-manager-go` directory.

0. Make sure you have MongoDB up and running (on localhost)
1. Install Go 1.8 or newer
2. `export GOPATH=$FM`
3. `cd $FM/src/flamenco-manager`
4. Download all dependencies with `go get`
5. Download Flamenco test dependencies with `go get -t ./...`
6. Run the unittests with `go test ./...`
7. Build your first Flamenco Manager with `go install`; this will create an executable
   `flamenco-manager` in `$FM/bin`
8. Copy `flamenco-manager-example.yaml` and name it `flamenco-manager.yaml` and then update
   it with the info generated after creating a manager document on the Server
9. Run the Manager with `../../bin/flamenco-manager -verbose`


### Testing

To run all unit tests, run `go test ./... -v`. To run a specific GoCheck test, run
`go test ./flamenco -v --run TestWithGocheck -check.f SchedulerTestSuite.TestVariableReplacement`
where the argument to `--run` determines which suite to run, and `-check.f` determines the
exact test function of that suite. Once all tests have been moved over to use GoCheck, the
`--run` parameter will probably not be needed any more.


## Communication between Server and Manager

Flamenco Manager is responsible for initiating all communication between Server and Manager,
since Manager should be able to run behind some firewall/router, without being reachable by Server.

In the text below, `some_fields` refer to configuration file settings.

### Fetching tasks

1. When a Worker ask for a task, it is served a task in state `queued` or `claimed-by-manager` in
   the local task queue (MongoDB collection "flamenco_tasks"). In this case, Manager performs a
   conditional GET (based on etag) to Server at /api/flamenco/tasks/{task-id} to see if the task
   has been updated since queued. If this is so, the task is updated in the queue and the queue
   is re-examined.
2. When the queue is empty, the manager fetches N new tasks from the Server, where N is the number
   of registered workers.

### Task updates and canceling running tasks

0. Pushes happen as POST to "/api/flamenco/managers/{manager-id}/task-update-batch"
1. Task updates queued by workers are pushed every `task_update_push_max_interval_seconds`, or
   when `task_update_push_max_count` updates are queued, whichever happens sooner.
2. An empty list of task updates is pushed every `cancel_task_fetch_max_interval_seconds`, unless an
   actual push (as described above) already happened within that time.
3. The response to a push contains the database IDs of the accepted task updates, as well as
   a list of task database IDs of tasks that should be canceled. If this list is non-empty, the
   tasks' statuses are updated accordingly.


## Timeouts of active tasks

When a worker starts working on a task, that task moves to status "active". The worker then
regularly calls `/may-i-run/{task-id}` to verify that it is still allowed to run that task. If this
end-point is not called within `active_task_timeout_interval_seconds` seconds, it will go to status
"failed". The default for this setting is 60 seconds, which is likely to be too short, so please
configure it for your environment.

This timeout check will start running 5 minutes after the Manager has started up. This allows
workers to let it know they are still alive, in case the manager was unreachable for longer than
the timeout period. For now this startup delay is hard-coded.


## Known issues & limitations

1. The downloading of tasks doesn't consider job types. This means that workers can be starved
   waiting for tasks, when there are 1000nds of tasks and workers of type X and only a relatively
   low number of workers and tasks of type Y.

## MISSING FEATURES / TO DO

In no particular order:

- Task queue cleanup. At the moment tasks are stored in the queue forever, since that makes
  it possible to notice a task was canceled while a worker was running it. Eventually such
  tasks should be cleaned up, though.
- GZip compression on the pushes to Server. This is especially important for task updates, since
  they contain potentially very large log entries.
- A way for Flamenco Server to get an overview of Workers, and set their status.
- the Task struct in `documents.go` should be synced with the Eve schema.
