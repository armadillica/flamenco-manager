Changelog for Flamenco Manager
==============================

## Version 2.0.2 (released 2017-03-31)

- Added `/output-produced` endpoint, where Workers can register the files
  they have produced (e.g. rendered images). This allows the "last rendered
  image" feature to work on filesystems that don't support inotify as well.


## Version 2.0.1 (released 2017-03-30)

- Added display of last-rendered image in a certain directory. Use
  the `watch_for_latest_image` configuration file option to enable this.


## Version 2.0

- Initial version for this changelog.
