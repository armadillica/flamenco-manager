# Flamenco Manager

This is the Flamenco Manager implementation in Go.

Author: Sybren A. Stüvel <sybren@blender.studio>


## Getting started

To run Flamenco Manager for the first time, follow these steps:

1. Install [ImageMagick](https://www.imagemagick.org/script/download.php) and make sure that the
   `convert` command can be found on `$PATH`.
2. If you don't want to use the bundled MongoDB server, download
   [MongoDB Community Server](https://www.mongodb.com/download-center/community)
   and install it. This is recommended on Windows as it seems to improve stability.
3. Download [Flamenco Manager](https://www.flamenco.io/download/) for your platform.
4. Extract the downloaded file.
5. Run `./flamenco-manager -setup` (Linux/macOS) or `flamenco-manager.exe -setup` (Windows).
6. Flamenco Manager will give you a list of URLs at which it can be reached. Open the URL that is
   reachable both for you and the workers.
7. Link Flamenco Manager to Blender Cloud by following the steps in the web interface.
8. Configure Flamenco Manager via the web interface. Update the variables and path replacement
   variables for your render farm; the `blender` variable should point to the Blender executable
   where it can be found *on the workers*, and similar for the `ffmpeg` variable.
   The path replacement variables allow you to set different paths for both Clients (like the
   Blender Cloud Add-on) and Workers, given their respective platforms.
9. Once you have completed configuration, restart Flamenco Manager through the web interface. It
   will now run in normal (i.e. non-setup) mode.

Note that `variables` and `path_replacement` share a namespace -- variable names have to be unique,
and cannot be used in both `variables` and `path_replacement` sections. If this happens, Flamenco
Manager will log the offending name, and refuse to start.


## Version numbers

Released versions of Flamenco Manager have a version number `v{major}.{minor}`, like `v2.4`,
or `v{major}.{minor}.{fix}`, like `v2.4.1`.

Development versions have `v{release}-{number}-{hash}`, where `number` indicates the
number of commits since the official version `v{release}`. The `hash` is the Git hash
of the last commit. If the version number ends with `-dirty` it means that there were
uncommitted changes when Flamenco Manager was built.


## Advanced Configuration

Apart from the above web-based setup, you can configure advanced settings by editing
`flamenco-manager.yaml`. For example, you can:

- Manager secure communication (see the next section).
- Set intervals for various periodic operations. See `flamenco-manager-example.yaml` for a
  description.

Intervals (like `download_task_sleep`) can be configured in seconds, minutes, or hours, by appending
a suffix `s`, `m`, or `h`. Such a suffix must always be used.


## HTTPS with custom TLS certificates or ACME/Let's Encrypt

To secure web traffic using HTTPS, we recommend using either Let's Encrypt or custom TLS
certificates. Transport Layer Security (TLS) is the we-are-no-longer-living-in-the-90ies name for
SSL.

Let's Encrypt can be used when the machine is publicly reachable and has a valid domain name. This
is easy to set up, as it automatically requests & renews certificates:
- Set `acme_domain_name` to the domain name of the machine.
- Set both `listen` and `listen_https` to the ports Flamenco Manager should be listening to. By
  default these are `:8080` and `:8443`.
- Configure your firewall or user-facing proxy to forward ports 80 and 443 to respectively 8080 and
  8443.

If you want to manage your own TLS certificates, set the path in the `tlskey` and `tlscert`
configuration options. Then set `listen_https` to the appropriate port number.


## CLI arguments

Flamenco Manager accepts the following CLI arguments:

- `-setup`: Start in *setup mode*, which will enable the web-based setup on the `/setup` URL.
- `-debug`: Enable debug-level logging
- `-quiet`: Disable info-level logging (no-op if `-debug` is also given), so that only warnings
  and errors are logged.
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
1. Install Go 1.12 or newer and GNU Make.
2. `export GOPATH=/path/to/your/workspace/for/go`
3. `cd $FM`
4. Install "dep" with `go get -u github.com/golang/dep/cmd/dep`
5. Download all dependencies with `dep ensure`
6. Download Flamenco test dependencies with `go get -t ./...`
7. Run the unittests with `make test`
8. Build your first Flamenco Manager with `make`; this will create an executable
   `flamenco-manager` in the current directory.
9. Configure Flamenco Manager by starting it in *setup mode* (`./flamenco-manager -setup`, see above).
10. Run the Manager with `./flamenco-manager`.


### Testing

To run all unit tests, run `make test`. To run a specific GoCheck test, run
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

The distributable Flamenco Manager packages are built using GNU Make.

1. Install GNU Make for your platform.
2. Prepare the bundled MongoDB server files:
    - [Download MongoDB](https://www.mongodb.com/download-center?jmp=nav#community)
      for Linux (the "legacy" build), Windows (the "2008 and later without SSL" version), and MacOS
      (the version without SSL). Versions without SSL support are used because they're simpler and
      we listen on localhost anyway so SSL is not necessary.
    - Extract the files you downloaded (the Windows version may require `msiextract` from the
      `msitools` package if you're extracting on Linux).
    - Make sure the contents can be found in `dist/mongodb-{linux-x86_64,osx-x86_64,windows}-version`,
      so the Linux `bin` directory should be in `dist/mongodb-{linux-x86_64,osx-x86_64,windows}-version/bin`.
    - Remove everything from the `bin` directories except `mongod` (or `mongod.exe` for the Windows
      version).
3. Run `make package` to create the distributable packages.


## Worker Sleep Schedule

Each worker has a sleep schedule, which can be configured from the dashboard and behaves in the
following way:

- When the schedule is not active, it doesn't influence the worker at all.
- When the schedule is active, the worker is requested to be active by default, unless the schedule
  allows it to sleep.
- When the schedule has 'days of week' set to a non-empty string, the worker is only sent to sleep
  on those days. When 'days of week' is empty, the worker is allowed to sleep on every day.
  The 'days of week' should be a space-separated list of the first two letters of the days, like
  `"mo tu we"`.
- The worker is allowed to sleep between the schedule's start and end time. Those times default to
  respectively the start and end of the day.
