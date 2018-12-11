Changelog for Flamenco Manager
==============================

## Version 2.3 (in development)

- Fix an issue where a task could time out when its Worker goes to sleep or shuts down.
- Upgraded the web interface to use Bootstrap 4.
- Dashboard now uses Vue.js for a dynamic interface.
- Dashboard drops support for Internet Explorer 11 due to it not supporting modern JavaScript.
- Workers can be selected and sent actions with one button click.


## Version 2.2 (2018-12-04)

- Requires Flamenco Server 2.1 or newer.
- Accept log entries for tasks that are no longer runnable. In this case the task's status and
  activity doesn't change, but the logs are still accepted & forwarded to Flamenco Server. This
  helps to figure out why a task failed, even when the logs are lagging behind.
- Include upstream queue size on dashboard.
- Send the set of task types supported by our workers to Flamenco Server. This will allow it to
  tailor some variable jobs to our capabilities.
- Allow Workers to return tasks to the queue.
- Store log entries in local files on the Manager, instead of sending all of them to the Server.
  The log files are stored in a directory per job, and a file per task. When a task is restarted,
  its log file is rotated (`{task-id}.log` becomes `{task-id}.log.1`). There is no automatic
  cleanup of log files implemented; this can be handled by a system daemon or by manual deletion.
  Log files can be accessed at http://manager/logfile/{job-id}/{task-id}. Requires Flamenco Server
  version 2.1 or newer.


## Version 2.1.1 (2018-01-21)

- Fixed race condition in JavaScript loading.
- Fixed incompatibility of "latest image" server-side event system with Firefox.
- Limit display height of last-rendered image to 300 pixels.
- Added `job_storage` path replacement variable to default configuration.
- Log a warning when backslashes are used in path replacement variables. Those should not be used,
  but forward slashes should be used for every platform.
- Allow erasing idle workers from the dashboard.
- Show 'last seen' timestamp in idle workers tooltip on the dashboard.
- Built with Golang 1.10


## Version 2.1.0 (2018-01-04)

- Added ability to send workers to sleep (and wake them up again) to the dashboard. This is done via
  a request to change its internal state. This state change must be acknowleged by the Worker before
  new tasks will be given. This is a backward-incompatible change, and requires you to upgrade your
  Workers to version 2.1.x or newer.
- Always log which version of Flamenco is running.
- Added a note that the MongoDB files should reside on a local filesystem, and not on a network.
- Prevent squashing of last-rendered image.
- Refuse task updates for tasks in non-runnable state. This means that once a task is cancelled,
  completed, etc. the worker cannot update it any more.
- Log activity when task gets cancelled by request of Flamenco Server.
- Tweaked the colour scheme of the web interface to be a bit more muted and easier to read.
- Limit latest image system queue to 3 images, and discard newer ones until the queue shrinks.
- Scale latest images down to max full HD size (maintains aspect ratio).
- Renamed worker status "down" to "offline"
- Added support for testing Workers. This test requires Worker version 2.1.0+, and requires that the
  worker is started with the `--test` CLI argument. For more details see the Flamenco documentation.


## Version 2.0.15 (released 2017-09-09)

- Flamenco Manager can now be run from a different directory than the executable is in. It searches
  for web templates both relative to the current working directory and relative to the executable's
  own directory.


## Version 2.0.14 (released 2017-09-07)

- Fixed panic when enabling UPnP/SSDP auto-discovery on Windows.
- Bundled MongoDB itself with Flamenco Manager, so that a separate MongoDB installation is no longer
  required. When the `database_url` in `flamenco-manager.yaml` is empty, this bundled database
  server will be used. Note that running unit tests while developing Flamenco Manager still requires
  a separate server instance.
- Added web interface for configuring Flamenco Manager. Start it using `flamenco-manager -setup`.
- Restyled the dashboard.


## Version 2.0.13 (released 2017-07-04)

- Added auto-discovery via UPnP/SSDP, so that Workers can automatically find this Manager on the
  network. This can be turned off by setting `ssdp_discovery` to `false`. For now only discovery via
  IPv4 is supported; this deficiency is reported at https://github.com/fromkeith/gossdp/issues/4,
  and also for the alternative UPnP/SSDP implementation at https://github.com/koron/go-ssdp/issues/4


## Version 2.0.12 (released 2017-06-23)

- Added `-purgequeue` CLI command, which erases all queued task updates from the local MongoDB, then
  exits Flamenco Manager.
- The `-cleanslate` CLI command now exits immediately when there are no tasks locally cached, i.e.
  when it would be a no-op anyway.
- The `-purgequeue` and `-cleanslate` commands now show how many items they would erase, before
  asking the user for confirmation.


## Version 2.0.11 (released 2017-06-16)

- Fixed a compatibility issue with Windows 10.


## Version 2.0.10 (released 2017-06-09)

- Added --factory-startup option to example Blender CLI variable. This is needed because this
  option was removed from the hard-coded values in the Flamenco Worker.
- When worker asks for tasks, also check already assigned tasks.
  [T51519](https://developer.blender.org/T51519)
- Changes the way durations are stored in flamenco_manager.yaml. They must now be expressed
  including units with a suffix (h, m, or s), rather than having the units in the configuration
  variable name.

    OLD: download_task_sleep_seconds = 30
    NEW: download_task_sleep = 30s
- Introduced path replacement variables, to allow Clients and Workers to run on different platforms.


## Version 2.0.9 (released 2017-05-11)

- Workers: only store host part of worker's address, and not the port number.
- Dashboard: Shorten task and worker IDs and nicer timestamp formatting.


## Version 2.0.8 (released 2017-05-09)

- Use mutex in scheduler to avoid race condition.
- Clear the worker's current task upon sign-on. This makes the dasboard less confusing
  when the worker's task was rescheduled to another worker.
- On the dashboard, for the current/task of a worker, show the last timestamp/status
  that this particular worker worked on it (rather than showing the last timestamp/status
  of the task itself)
- Dashboard: Show workers as table (instead of blocks).
- Dashboard: click on worker ID or address to copy it to the clipboard.


## Version 2.0.7 (released 2017-04-26)

- Added 'Kick task downloader' button to dashboard.
- Dashboard: also update Manager version from JSON.


## Version 2.0.6 (released 2017-04-21)

- Reduced logging of workers requesting tasks, as we now have a nice dashboard
  to show the status.
- Reduced logging of task downloading.
- Not-seen workers are now moved to "Old workers" after 14 days (instead of 31)


## Version 2.0.5 (released 2017-04-20)

- Fixed race condition where a cancelled task could be re-activated
  by a worker.


## Version 2.0.4 (released 2017-04-07)

- Small dashboard JS tweak: hide workers we haven't seen in over a
  month.
- Added support for task types. Only tasks of a type that workers
  support will be scheduled for them. This also adds a /sign-on
  URL so workers can send a current list of supported task types
  (and their current nickname) to the Manager.
- Dashboard: Vertically align last-rendered image.


## Version 2.0.3 (released 2017-04-04)

- Implemented error checking for JSON encoding & sending.

  This should work around a timeout issue we've seen, where a worker
  times out while waiting for the scheduler. The Manager would ignore
  the error and keep the task assigned to the worker. Now it detects
  the error and unassigns the worker.


## Version 2.0.2 (released 2017-03-31)

- Added `/output-produced` endpoint, where Workers can register the files
  they have produced (e.g. rendered images). This allows the "last rendered
  image" feature to work on filesystems that don't support inotify as well.


## Version 2.0.1 (released 2017-03-30)

- Added display of last-rendered image in a certain directory. Use
  the `watch_for_latest_image` configuration file option to enable this.


## Version 2.0

- Initial version for this changelog.
