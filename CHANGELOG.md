Changelog for Flamenco Manager
==============================

## Version 2.0.10 (in development)

- Added --factory-startup option to example Blender CLI variable. This is needed because this
  option was removed from the hard-coded values in the Flamenco Worker.


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
