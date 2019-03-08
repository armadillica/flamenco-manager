# Ideas for the future

In no particular order:

- Remove testing endpoints (including the dummy JWT token generation).
- Monitor free harddisk space for checkout and file storage directories.
- Graceful shutdown:
    * Close HTTP server while keeping current requests running.
    * Complete currently-running checkouts.
    * Maybe complete currently running file uploads?
- Automatic cleanup of unfinished uploads.
