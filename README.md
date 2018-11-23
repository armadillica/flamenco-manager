# Flamenco Manager

This is the Flamenco Manager implementation in Go.

Author: Sybren A. Stüvel <sybren@blender.studio>


## Getting started

To run Flamenco Manager for the first time, follow these steps:

0. Install [ImageMagick](https://www.imagemagick.org/script/download.php) and make sure that the
   `convert` command can be found on `$PATH`.
1. Download [Flamenco Manager](https://www.flamenco.io/download/) for your platform.
2. Extract the downloaded file.
3. Run `./flamenco-manager -setup` (Linux/macOS) or `flamenco-manager.exe -setup` (Windows).
4. Flamenco Manager will give you a list of URLs at which it can be reached. Open the URL that is
   reachable both for you and the workers.
5. Link Flamenco Manager to Blender Cloud by following the steps in the web interface.
6. Configure Flamenco Manager via the web interface. Update the variables and path replacement
   variables for your render farm; the `blender` variable should point to the Blender executable
   where it can be found *on the workers*, and similar for the `ffmpeg` variable.
   The path replacement variables allow you to set different paths for both Clients (like the
   Blender Cloud Add-on) and Workers, given their respective platforms.
7. Once you have completed configuration, restart Flamenco Manager through the web interface. It
   will now run in normal (i.e. non-setup) mode.

Note that `variables` and `path_replacement` share a namespace -- variable names have to be unique,
and cannot be used in both `variables` and `path_replacement` sections. If this happens, Flamenco
Manager will log the offending name, and refuse to start.


## Advanced Configuration

Apart from the above web-based setup, you can configure advanced settings by editing
`flamenco-manager.yaml`. For example, you can:

- Generate TLS certificates and set the path in the `tlskey` and `tlscert` configuration
  options. Transport Layer Security (TLS) is the we-are-no-longer-living-in-the-90ies name for SSL.
- Set intervals for various periodic operations. See `flamenco-manager-example.yaml` for a
  description.

Intervals (like `download_task_sleep`) can be configured in seconds, minutes, or hours, by appending
a suffix `s`, `m`, or `h`. Such a suffix must always be used.


## CLI arguments

Flamenco Manager accepts the following CLI arguments:

- `-setup`: Start in *setup mode*, which will enable the web-based setup on the `/setup` URL.
- `-debug`: Enable debug-level logging
- `-verbose`: Enable info-level logging (no-op if `-debug` is also given). This is automatically
  enabled in setup mode.
- `-json`: Log in JSON format, instead of plain text
- `-cleanslate`: Start with a clean slate; erases all cached tasks from the local MongoDB,
  then exits Flamenco Manager. This can be run while another Flamenco Manager is
  running, but this scenario has not been well-tested yet.
- `-purgequeue`: Erases all queued task updates from the local MongoDB, then exits Flamenco Manager.
  NOTE: *this is a lossy operation*, and it may erase important task updates. Only perform this when
  you know what you're doing.


## Running as service via systemd (Linux-only)

1. Build (see below) and configure Flamenco Manager.
2. Edit `flamenco-manager.service` to update it for the installation location, then place the file
   in `/etc/systemd/system`.
3. Run `systemctl daemon-reload` to pick up on the new/edited file.
4. Run `systemctl start flamenco-manager` to start Flamenco Manager.
5. Run `systemctl enable flamenco-manager` to ensure it starts at boot too.


## Starting development

`$FM` denotes the directory containing a checkout of Flamenco Manager, that is, the absolute path
of this `flamenco-manager` directory.

0. Make sure you have MongoDB up and running (on localhost)
1. Install Go 1.9 or newer
2. `export GOPATH=/path/to/your/workspace/for/go`
3. `cd $FM`
4. Install "dep" with `go get -u github.com/golang/dep/cmd/dep`
5. Download all dependencies with `dep ensure`
6. Download Flamenco test dependencies with `go get -t ./...`
7. Run the unittests with `go test ./...`
8. Build your first Flamenco Manager with `go install`; this will create an executable
   `flamenco-manager` in `$GOPATH/bin`. It may be a good idea to add `$GOPATH/bin` to your `PATH`
   environment variable.
9. Configure Flamenco Manager by starting it in *setup mode* (`flamenco-manager -setup`, see above).
10. Run the Manager with `$GOPATH/bin/flamenco-manager -verbose`.


### Testing

To run all unit tests, run `go test ./... -v`. To run a specific GoCheck test, run
`go test ./flamenco -v --run TestWithGocheck -check.f SchedulerTestSuite.TestVariableReplacement`
where the argument to `--run` determines which suite to run, and `-check.f` determines the
exact test function of that suite. Once all tests have been moved over to use GoCheck, the
`--run` parameter will probably not be needed any more.


## Communication between Server and Manager

Flamenco Manager is responsible for initiating all communication between Server and Manager,
since Manager should be able to run behind some firewall/router, without being reachable by Server.

In the text below, `some_fields` can refer to configuration file settings.

### Fetching tasks

1. When a Worker ask for a task, it is served a task in state `queued` or `claimed-by-manager` in
   the local task queue (MongoDB collection "flamenco_tasks"). In this case, Manager performs a
   conditional GET (based on etag) to Server at /api/flamenco/tasks/{task-id} to see if the task
   has been updated since queued. If this is so, the task is updated in the queue and the queue
   is re-examined.
2. When the queue is empty, the manager fetches new tasks from the Server. This is also done when
   one clicks the "Kick task downloader" button in the dashboard.


### Task updates and canceling running tasks

0. Pushes happen as POST to "/api/flamenco/managers/{manager-id}/task-update-batch"
1. Task updates queued by workers are pushed every `task_update_push_max_interval`, or
   when `task_update_push_max_count` updates are queued, whichever happens first.
2. An empty list of task updates is pushed every `cancel_task_fetch_max_interval`, unless an
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


## Missing features / future work

In no particular order:

- GZip compression on the pushes to Server. This is especially important for task updates, since
  they contain potentially very large log entries.
- A way for Flamenco Server to get an overview of Workers, and set their status.


## Building distributable packages

The distributable Flamenco Manager packages are built using Docker. This allows us to build a
static binary without impacting the locally installed version of Go. The process is as follows:

1. Install [Docker Community Edition](https://www.docker.com/community-edition).
2. `cd` into the `docker` directory.
3. Prepare the bundled MongoDB server files:
    - [Download MongoDB](https://www.mongodb.com/download-center?jmp=nav#community)
      for Linux (the "legacy" build), Windows (the "2008 and later without SSL" version), and MacOS
      (the version without SSL). Versions without SSL support are used because they're simpler and
      we listen on localhost anyway so SSL is not necessary.
    - Extract the files you downloaded (the Windows version may require `msiextract` from the
      `msitools` package if you're extracting on Linux).
    - Make sure the contents can be found in `docker/mongodb-{linux,osx,windows}-version`,
      so the Linux `bin` directory should be in `docker/mongodb-{linux,osx,windows}-version/bin`.
    - Remove everything from the `bin` directories except `mongod` (or `mongod.exe` for the Windows
      version).
4. Run `./build-via-docker.sh` to create the distributable packages.
