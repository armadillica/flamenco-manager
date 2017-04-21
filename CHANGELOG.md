Changelog for Flamenco Manager
==============================

## Version 2.0.6 (released 2017-04-21)

- Reduced logging of workers requesting tasks, as we now have a nice dashboard
  to show the status.
- Reduced logging of task downloading.


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
